package com.edgeaware.app.data

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

sealed interface SyncStatus {
    data object Idle : SyncStatus
    data object Syncing : SyncStatus
    data object Offline : SyncStatus
    data object NeedsLogin : SyncStatus
    data class Failed(val message: String) : SyncStatus
}

/**
 * Brings the local copy and the server together: first push this device's queued
 * changes, then pull everything new. Safe to call often; only one sync runs at a time.
 */
class SyncEngine(
    private val api: ApiClient,
    private val repo: TaskRepository,
    private val session: SessionStore,
    private val deviceName: String,
) {
    private val mutex = Mutex()

    private val _status = MutableStateFlow<SyncStatus>(SyncStatus.Idle)
    val status: StateFlow<SyncStatus> = _status.asStateFlow()

    // One-off messages for the user, e.g. "couldn't update X".
    private val _notices = MutableSharedFlow<String>(extraBufferCapacity = 16)
    val notices: SharedFlow<String> = _notices.asSharedFlow()

    suspend fun sync(): SyncStatus = mutex.withLock {
        _status.value = SyncStatus.Syncing
        val result = try {
            push()
            pull()
            repo.markSynced()
            SyncStatus.Idle
        } catch (e: CancellationException) {
            throw e
        } catch (e: UnauthorizedException) {
            SyncStatus.NeedsLogin
        } catch (e: OfflineException) {
            SyncStatus.Offline
        } catch (e: Exception) {
            SyncStatus.Failed(e.message ?: "Sync failed")
        }
        _status.value = result
        result
    }

    private suspend fun push() {
        // A conflict is retried once on top of the server's newer version: operators
        // only change status, so the server's other edits (title, due date) are kept
        // and the status change still goes through.
        for (attempt in 0..1) {
            val ops = repo.pendingOps()
            if (ops.isEmpty()) return
            val response = api.push(
                PushRequest(
                    deviceId = session.deviceId,
                    deviceName = deviceName,
                    changes = ops.map {
                        PushChange(it.op_id, "upsert", it.task_id, it.base_version, StatusFields(it.status))
                    },
                ),
            )
            val byId = ops.associateBy { it.op_id }
            var retry = false
            for (r in response.results) {
                val op = byId[r.opId] ?: continue
                when (r.status) {
                    "applied" -> repo.transaction {
                        repo.deleteOp(r.opId)
                        r.task?.let { repo.saveServerTask(it) }
                    }
                    "conflict" -> {
                        val server = r.task
                        if (attempt == 0 && server != null && server.assignedTo == session.userId) {
                            repo.transaction {
                                repo.rebaseOp(op.task_id, server.version)
                                repo.saveServerTask(server)
                            }
                            retry = true
                        } else {
                            notice("Couldn't update \"${repo.title(op.task_id)}\": it was changed on the server.")
                            repo.transaction {
                                repo.deleteOp(r.opId)
                                server?.let { repo.saveServerTask(it) }
                            }
                        }
                    }
                    "not_found" -> {
                        notice("\"${repo.title(op.task_id)}\" no longer exists.")
                        repo.transaction { repo.removeTask(op.task_id) }
                    }
                    "forbidden" -> {
                        notice("You're not allowed to update \"${repo.title(op.task_id)}\".")
                        repo.deleteOp(r.opId)
                    }
                    else -> {
                        notice("Couldn't update \"${repo.title(op.task_id)}\": ${r.error ?: r.status}.")
                        repo.deleteOp(r.opId)
                    }
                }
            }
            if (!retry) return
        }
    }

    private suspend fun pull() {
        var cursor = repo.cursor()
        while (true) {
            val page = try {
                api.pull(cursor, session.deviceId)
            } catch (e: CursorExpiredException) {
                // Offline too long: the server cleaned up what we missed. Start fresh.
                val snap = api.snapshot(session.deviceId)
                repo.replaceAll(snap.tasks, snap.cursor)
                return
            }
            repo.transaction {
                for (c in page.changes) {
                    val task = c.task
                    if (c.kind == "upsert" && task != null) repo.saveServerTask(task) else repo.removeTask(c.taskId)
                }
                repo.setCursor(page.nextCursor)
            }
            cursor = page.nextCursor
            if (!page.hasMore) return
        }
    }

    private fun notice(message: String) {
        _notices.tryEmit(message)
    }
}
