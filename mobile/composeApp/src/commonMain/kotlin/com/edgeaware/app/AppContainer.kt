package com.edgeaware.app

import com.edgeaware.app.data.ApiClient
import com.edgeaware.app.data.SessionStore
import com.edgeaware.app.data.SyncEngine
import com.edgeaware.app.data.TaskRepository
import com.edgeaware.app.db.EdgeAwareDb
import com.edgeaware.app.platform.PlatformDeps

/** Builds the app's objects once, from the platform-specific pieces. */
class AppContainer(deps: PlatformDeps) {
    val session = SessionStore(deps.keyValueStore)
    val repo = TaskRepository(EdgeAwareDb(deps.createSqlDriver()))
    val api = ApiClient(deps.createHttpEngine(), session)
    val sync = SyncEngine(api, repo, session, deps.deviceName)
}
