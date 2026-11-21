package com.edgeaware.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.edgeaware.app.AppContainer
import com.edgeaware.app.data.ApiException
import com.edgeaware.app.data.OfflineException
import com.edgeaware.app.data.SyncStatus
import com.edgeaware.app.db.Sync_state
import com.edgeaware.app.db.VisibleTask
import com.edgeaware.app.db.VisibleTasks
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class LoginUiState(
    val serverUrl: String,
    val username: String,
    val loading: Boolean = false,
    val error: String? = null,
)

class AppViewModel(private val c: AppContainer) : ViewModel() {

    private val _loggedIn = MutableStateFlow(c.session.isLoggedIn)
    val loggedIn: StateFlow<Boolean> = _loggedIn.asStateFlow()

    private val _login = MutableStateFlow(LoginUiState(c.session.serverUrl, c.session.username ?: ""))
    val login: StateFlow<LoginUiState> = _login.asStateFlow()

    val username: String get() = c.session.username ?: ""

    val tasks: StateFlow<List<VisibleTasks>> =
        c.repo.tasks().stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), emptyList())
    val pendingCount: StateFlow<Long> =
        c.repo.pendingCount().stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), 0)
    val syncState: StateFlow<Sync_state?> =
        c.repo.syncState().stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)
    val syncStatus: StateFlow<SyncStatus> = c.sync.status
    val notices: SharedFlow<String> = c.sync.notices

    fun task(id: String): Flow<VisibleTask?> = c.repo.task(id)

    private var autoSync: Job? = null

    init {
        if (_loggedIn.value) startAutoSync()
    }

    fun login(serverUrl: String, username: String, password: String) {
        if (username.isBlank() || password.isBlank()) {
            _login.update { it.copy(error = "Enter your username and password") }
            return
        }
        _login.update { it.copy(serverUrl = serverUrl, username = username, loading = true, error = null) }
        viewModelScope.launch {
            try {
                c.session.serverUrl = serverUrl
                c.session.token = c.api.login(username.trim(), password)
                val me = c.api.me()
                // A different person on this device: don't show them the last user's tasks.
                if (c.session.userId != null && c.session.userId != me.id) c.repo.clearAll()
                c.session.userId = me.id
                c.session.username = me.username
                _login.update { it.copy(loading = false) }
                _loggedIn.value = true
                startAutoSync()
            } catch (e: CancellationException) {
                throw e
            } catch (e: OfflineException) {
                c.session.token = null
                _login.update { it.copy(loading = false, error = "Can't reach the server at $serverUrl") }
            } catch (e: ApiException) {
                c.session.token = null
                _login.update { it.copy(loading = false, error = e.message) }
            }
        }
    }

    fun setStatus(taskId: String, status: String) {
        c.repo.setStatus(taskId, status)
        syncNow()
    }

    fun syncNow() {
        viewModelScope.launch { runSync() }
    }

    /** Logs out but keeps the local data, so queued changes are sent after the next login. */
    fun logout() {
        autoSync?.cancel()
        c.session.token = null
        _loggedIn.value = false
    }

    private suspend fun runSync() {
        if (c.sync.sync() == SyncStatus.NeedsLogin) {
            logout()
            _login.update { it.copy(error = "Your session expired. Log in again to sync.") }
        }
    }

    private fun startAutoSync() {
        autoSync?.cancel()
        autoSync = viewModelScope.launch {
            while (true) {
                runSync()
                delay(30_000)
            }
        }
    }
}
