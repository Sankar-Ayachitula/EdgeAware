package com.edgeaware.app

import app.cash.sqldelight.driver.jdbc.sqlite.JdbcSqliteDriver
import com.edgeaware.app.data.ApiClient
import com.edgeaware.app.data.ChangeDto
import com.edgeaware.app.data.OpResult
import com.edgeaware.app.data.PullResponse
import com.edgeaware.app.data.PushRequest
import com.edgeaware.app.data.PushResponse
import com.edgeaware.app.data.SessionStore
import com.edgeaware.app.data.SnapshotResponse
import com.edgeaware.app.data.SyncEngine
import com.edgeaware.app.data.SyncStatus
import com.edgeaware.app.data.TaskDto
import com.edgeaware.app.data.TaskRepository
import com.edgeaware.app.data.TaskStatus
import com.edgeaware.app.db.EdgeAwareDb
import com.edgeaware.app.platform.KeyValueStore
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.OutgoingContent
import io.ktor.http.headersOf
import io.ktor.utils.io.errors.IOException
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

private const val ME = "11111111-1111-1111-1111-111111111111"
private const val BOSS = "22222222-2222-2222-2222-222222222222"
private const val OTHER = "33333333-3333-3333-3333-333333333333"

private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

/** A tiny in-memory stand-in for the backend's sync endpoints. */
private class FakeServer {
    val tasks = mutableMapOf<String, TaskDto>()
    val feed = mutableListOf<ChangeDto>()
    var prunedThrough = 0L
    var offline = false
    var unauthorized = false
    var pageSize = 2
    val pushes = mutableListOf<PushRequest>()

    /** Called before each push change; return a non-null result to override (e.g. forbidden). */
    var pushOverride: (PushRequest, Int) -> OpResult? = { _, _ -> null }

    private var seq = 0L

    fun put(t: TaskDto) {
        tasks[t.id] = t
        feed += ChangeDto(++seq, "upsert", t.id, t)
    }

    fun revoke(id: String) {
        tasks.remove(id)
        feed += ChangeDto(++seq, "revoked", id)
    }

    fun handle(scope: MockRequestHandleScope, req: HttpRequestData): HttpResponseData = with(scope) {
        if (offline) throw IOException("no network")
        if (unauthorized) return respond("", HttpStatusCode.Unauthorized)
        when (req.url.encodedPath) {
            "/sync/pull" -> {
                val since = req.url.parameters["since"]!!.toLong()
                if (since < prunedThrough) return respond("""{"error":"cursor_expired"}""", HttpStatusCode.Gone)
                val after = feed.filter { it.seq > since }
                val page = after.take(pageSize)
                // Like the server: return the current copy of upserted tasks.
                val changes = page.map { c -> if (c.kind == "upsert") c.copy(task = tasks[c.taskId]) else c }
                ok(PullResponse(changes, page.lastOrNull()?.seq ?: since, after.size > page.size))
            }
            "/sync/snapshot" -> ok(SnapshotResponse(tasks.values.toList(), seq))
            "/sync/push" -> {
                val body = json.decodeFromString<PushRequest>((req.body as OutgoingContent.ByteArrayContent).bytes().decodeToString())
                pushes += body
                val results = body.changes.mapIndexed { i, c ->
                    pushOverride(body, i) ?: run {
                        val cur = tasks[c.taskId]
                        when {
                            cur == null -> OpResult(c.opId, "not_found")
                            cur.version != c.baseVersion -> OpResult(c.opId, "conflict", task = cur)
                            else -> {
                                val updated = cur.copy(status = c.fields.status, version = cur.version + 1)
                                put(updated)
                                OpResult(c.opId, "applied", task = updated)
                            }
                        }
                    }
                }
                ok(PushResponse(results))
            }
            else -> respond("", HttpStatusCode.NotFound)
        }
    }

    private inline fun <reified T> MockRequestHandleScope.ok(body: T) =
        respond(json.encodeToString(body), HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))
}

private class MemoryStore : KeyValueStore {
    private val map = mutableMapOf<String, String>()
    override fun get(key: String) = map[key]
    override fun put(key: String, value: String) { map[key] = value }
    override fun remove(key: String) { map.remove(key) }
}

private fun task(id: String, title: String, version: Long = 1, status: String = TaskStatus.TODO, assignee: String = ME) =
    TaskDto(id = id, title = title, status = status, assignedTo = assignee, createdBy = BOSS, version = version)

class SyncEngineTest {
    private lateinit var server: FakeServer
    private lateinit var repo: TaskRepository
    private lateinit var engine: SyncEngine

    @BeforeTest
    fun setUp() {
        server = FakeServer()
        val driver = JdbcSqliteDriver(JdbcSqliteDriver.IN_MEMORY).also { EdgeAwareDb.Schema.create(it) }
        val session = SessionStore(MemoryStore()).apply {
            serverUrl = "http://test"
            token = "t"
            userId = ME
        }
        val api = ApiClient(MockEngine { req -> server.handle(this, req) }, session)
        repo = TaskRepository(EdgeAwareDb(driver))
        engine = SyncEngine(api, repo, session, "test device")
    }

    private suspend fun visible() = repo.tasks().first()

    @Test
    fun pullPagesThroughFeedAndAppliesRevokes() = runTest {
        repeat(5) { server.put(task("t$it", "Task $it")) }
        server.revoke("t3")

        assertEquals(SyncStatus.Idle, engine.sync())
        assertEquals(listOf("t0", "t1", "t2", "t4"), visible().map { it.id }.sorted())
        assertEquals(6, repo.cursor())

        // Nothing new: cursor stays, nothing changes.
        engine.sync()
        assertEquals(6, repo.cursor())
    }

    @Test
    fun localStatusChangeIsShownImmediatelyAndPushed() = runTest {
        server.put(task("t1", "Inspect valve"))
        engine.sync()

        repo.setStatus("t1", TaskStatus.IN_PROGRESS)
        repo.setStatus("t1", TaskStatus.DONE) // collapses into the same pending change
        val shown = visible().single()
        assertEquals(TaskStatus.DONE, shown.status)
        assertTrue(shown.pending)
        assertEquals(1, repo.pendingOps().size)

        engine.sync()
        assertEquals(1, server.pushes.size)
        assertEquals(TaskStatus.DONE, server.tasks["t1"]!!.status)
        val after = visible().single()
        assertEquals(TaskStatus.DONE, after.status)
        assertFalse(after.pending)
        assertEquals(2, after.version)
    }

    @Test
    fun changingBackToServerStatusCancelsThePendingChange() = runTest {
        server.put(task("t1", "Valve"))
        engine.sync()
        repo.setStatus("t1", TaskStatus.DONE)
        repo.setStatus("t1", TaskStatus.TODO)
        assertTrue(repo.pendingOps().isEmpty())
        engine.sync()
        assertTrue(server.pushes.isEmpty())
    }

    @Test
    fun offlineKeepsChangesQueuedUntilTheServerIsBack() = runTest {
        server.put(task("t1", "Valve"))
        engine.sync()
        server.offline = true
        repo.setStatus("t1", TaskStatus.IN_PROGRESS)

        assertEquals(SyncStatus.Offline, engine.sync())
        assertEquals(1, repo.pendingOps().size)
        assertEquals(TaskStatus.IN_PROGRESS, visible().single().status)

        server.offline = false
        assertEquals(SyncStatus.Idle, engine.sync())
        assertTrue(repo.pendingOps().isEmpty())
        assertEquals(TaskStatus.IN_PROGRESS, server.tasks["t1"]!!.status)
    }

    @Test
    fun conflictIsRebasedOntoServerVersionAndRetried() = runTest {
        server.put(task("t1", "Valve"))
        engine.sync()
        repo.setStatus("t1", TaskStatus.DONE)
        // Meanwhile the supervisor renamed it on the server (version 2).
        server.put(task("t1", "Valve 7 (urgent)", version = 2))

        assertEquals(SyncStatus.Idle, engine.sync())
        assertEquals(2, server.pushes.size) // conflict, then retry
        val t = server.tasks["t1"]!!
        assertEquals("Valve 7 (urgent)", t.title)
        assertEquals(TaskStatus.DONE, t.status)
        assertEquals(3, t.version)
        assertTrue(repo.pendingOps().isEmpty())
        assertEquals("Valve 7 (urgent)", visible().single().title)
    }

    @Test
    fun forbiddenChangeIsDroppedAndTheUserIsTold() = runTest {
        server.put(task("t1", "Valve"))
        engine.sync()
        repo.setStatus("t1", TaskStatus.DONE)
        server.pushOverride = { req, i -> OpResult(req.changes[i].opId, "forbidden") }

        val collected = mutableListOf<String>()
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) { engine.notices.collect { collected += it } }
        engine.sync()

        assertTrue(repo.pendingOps().isEmpty())
        assertEquals(TaskStatus.TODO, visible().single().status) // reverted to the server's value
        assertEquals(1, collected.size)
        assertTrue(collected.single().contains("Valve"))
    }

    @Test
    fun reassignedTaskDisappearsFromThePhone() = runTest {
        server.put(task("t1", "Valve"))
        engine.sync()
        repo.setStatus("t1", TaskStatus.IN_PROGRESS)
        // Supervisor gives it to someone else before we sync: our push conflicts with a
        // copy assigned to another user, so it is not retried, and the pull revokes it.
        server.put(task("t1", "Valve", version = 2, assignee = OTHER))
        server.revoke("t1")
        server.tasks["t1"] = task("t1", "Valve", version = 2, assignee = OTHER)

        engine.sync()
        assertTrue(visible().isEmpty())
        assertTrue(repo.pendingOps().isEmpty())
        assertEquals(1, server.pushes.size)
    }

    @Test
    fun expiredSessionAsksForLoginAndKeepsLocalChanges() = runTest {
        server.put(task("t1", "Valve"))
        engine.sync()
        repo.setStatus("t1", TaskStatus.DONE)
        server.unauthorized = true

        assertEquals(SyncStatus.NeedsLogin, engine.sync())
        assertEquals(1, repo.pendingOps().size)
    }

    @Test
    fun expiredCursorDownloadsSnapshot() = runTest {
        server.put(task("t1", "Old"))
        server.put(task("t2", "Kept"))
        engine.sync()
        // Long offline: t1 reassigned away, t3 added, and the feed was cleaned up.
        server.tasks.remove("t1")
        server.put(task("t3", "New"))
        server.prunedThrough = 100

        assertEquals(SyncStatus.Idle, engine.sync())
        assertEquals(listOf("Kept", "New"), visible().map { it.title }.sorted())
        assertEquals(3, repo.cursor())
    }

    @Test
    fun firstSyncWithNoTasksIsFine() = runTest {
        assertEquals(SyncStatus.Idle, engine.sync())
        assertTrue(visible().isEmpty())
        assertNull(repo.pendingOps().firstOrNull())
    }
}
