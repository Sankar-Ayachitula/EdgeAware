package com.edgeaware.app.data

import app.cash.sqldelight.coroutines.asFlow
import app.cash.sqldelight.coroutines.mapToList
import app.cash.sqldelight.coroutines.mapToOne
import app.cash.sqldelight.coroutines.mapToOneOrNull
import com.edgeaware.app.db.EdgeAwareDb
import com.edgeaware.app.db.Sync_state
import com.edgeaware.app.db.VisibleTask
import com.edgeaware.app.db.VisibleTasks
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlin.time.Clock
import kotlin.time.ExperimentalTime
import kotlin.uuid.ExperimentalUuidApi
import kotlin.uuid.Uuid

/**
 * The phone's local copy of the user's tasks. The UI reads only from here, so the
 * app works fully offline; SyncEngine keeps it in step with the server.
 */
class TaskRepository(private val db: EdgeAwareDb) {
    private val q get() = db.edgeAwareQueries

    init {
        q.initSyncState()
    }

    fun tasks(): Flow<List<VisibleTasks>> = q.visibleTasks().asFlow().mapToList(Dispatchers.Default)

    fun task(id: String): Flow<VisibleTask?> = q.visibleTask(id).asFlow().mapToOneOrNull(Dispatchers.Default)

    fun pendingCount(): Flow<Long> = q.pendingCount().asFlow().mapToOne(Dispatchers.Default)

    fun syncState(): Flow<Sync_state> = q.syncState().asFlow().mapToOne(Dispatchers.Default)

    /**
     * Changes a task's status on this device. The change is queued in the outbox and
     * shown immediately; SyncEngine sends it when the server is reachable. Several
     * edits before a sync collapse into one change against the same server version.
     */
    @OptIn(ExperimentalTime::class)
    fun setStatus(taskId: String, status: String) {
        require(status in TaskStatus.all) { "unknown status $status" }
        db.transaction {
            val server = q.serverTask(taskId).executeAsOneOrNull() ?: return@transaction
            val pending = q.pendingOpForTask(taskId).executeAsOneOrNull()
            when {
                status == server.status -> q.deleteOpForTask(taskId) // back to what the server has
                pending != null -> q.replaceOpStatus(newOpId(), status, taskId)
                else -> q.insertOp(newOpId(), taskId, server.version, status, Clock.System.now().toEpochMilliseconds())
            }
        }
    }

    // ----- used by SyncEngine -----

    internal fun pendingOps() = q.pendingOps().executeAsList()

    internal fun title(taskId: String): String =
        q.serverTask(taskId).executeAsOneOrNull()?.title ?: "a task"

    internal fun cursor(): Long = q.syncState().executeAsOne().cursor

    internal fun transaction(block: () -> Unit) = db.transaction { block() }

    internal fun saveServerTask(t: TaskDto) {
        q.upsertTask(t.id, t.title, t.description, t.status, t.dueAt, t.completedAt, t.assignedTo, t.createdBy, t.version, t.updatedAt)
        // If the user edited this task again while a sync was in flight, move that
        // pending edit onto the new server version (or drop it if it now matches).
        q.pendingOpForTask(t.id).executeAsOneOrNull()?.let { op ->
            if (op.status == t.status) q.deleteOpForTask(t.id)
            else if (op.base_version != t.version) q.rebaseOp(op.op_id, t.version, t.id)
        }
    }

    internal fun removeTask(taskId: String) {
        q.deleteTask(taskId)
        q.deleteOpForTask(taskId)
    }

    internal fun deleteOp(opId: String) = q.deleteOp(opId)

    /** Gives the pending change for this task a fresh op_id based on [version], so it is retried. */
    internal fun rebaseOp(taskId: String, version: Long) = q.rebaseOp(newOpId(), version, taskId)

    internal fun setCursor(cursor: Long) = q.setCursor(cursor)

    @OptIn(ExperimentalTime::class)
    internal fun markSynced() = q.setLastSync(Clock.System.now().toEpochMilliseconds())

    /** Replaces the whole local copy (after a snapshot download). */
    internal fun replaceAll(tasks: List<TaskDto>, cursor: Long) = db.transaction {
        q.deleteAllTasks()
        tasks.forEach { saveServerTask(it) }
        val ids = tasks.map { it.id }.toSet()
        q.pendingOps().executeAsList().filter { it.task_id !in ids }.forEach { q.deleteOp(it.op_id) }
        q.setCursor(cursor)
    }

    /** Wipes everything, e.g. when a different user logs in on this device. */
    fun clearAll() = db.transaction {
        q.deleteAllTasks()
        q.deleteAllOps()
        q.resetSyncState()
    }

    @OptIn(ExperimentalUuidApi::class)
    private fun newOpId() = Uuid.random().toString()
}
