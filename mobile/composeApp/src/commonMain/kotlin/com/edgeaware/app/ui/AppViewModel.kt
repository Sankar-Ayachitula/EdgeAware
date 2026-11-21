package com.edgeaware.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.edgeaware.app.AppContainer
import com.edgeaware.app.data.ApiException
import com.edgeaware.app.data.OfflineException
import com.edgeaware.app.data.SyncStatus
import com.edgeaware.app.data.TaskDto
import com.edgeaware.app.data.UnauthorizedException
import com.edgeaware.app.data.UserDto
import com.edgeaware.app.db.Sync_state
import com.edgeaware.app.db.VisibleTask
import com.edgeaware.app.db.VisibleTasks
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** What the profile tab shows; role comes from the last login and is refreshed when online. */
data class ProfileUiState(
    val username: String,
    val roleName: String?,
    val roleLevel: Int?,
    val serverUrl: String,
    val deviceName: String,
)

/** One person's page in the hierarchy tab: who reports to them and their tasks. */
data class PersonPage(val juniors: List<UserDto>, val tasks: List<TaskDto>)

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

    // Short "Synced" / "Server error" messages shown as a toast after a sync.
    private val _toasts = MutableSharedFlow<String>(extraBufferCapacity = 8)
    val toasts: SharedFlow<String> = _toasts.asSharedFlow()

    fun task(id: String): Flow<VisibleTask?> = c.repo.task(id)

    val myUserId: String get() = c.session.userId ?: ""

    private fun currentProfile() = ProfileUiState(
        username = c.session.username ?: "",
        roleName = c.session.roleName,
        roleLevel = c.session.roleLevel,
        serverUrl = c.session.serverUrl,
        deviceName = c.deviceName,
    )

    private val _profile = MutableStateFlow(currentProfile())
    val profile: StateFlow<ProfileUiState> = _profile.asStateFlow()

    /** Refreshes the role from the server; keeps the saved one when offline. */
    fun refreshProfile() {
        viewModelScope.launch {
            try {
                saveRole(c.api.me())
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
            }
            _profile.value = currentProfile()
        }
    }

    private fun saveRole(me: UserDto) {
        c.session.roleName = me.role.name
        c.session.roleLevel = me.role.level
    }

    /**
     * Loads a person's page for the hierarchy tab. This is read from the server (not the
     * offline store), so it needs a connection. Your own page has no tasks section: those
     * are on the My tasks tab.
     */
    suspend fun loadPerson(userId: String): Result<PersonPage> = try {
        val juniors = c.api.juniors(userId)
        val tasks = if (userId == myUserId) emptyList() else c.api.userTasks(userId)
        Result.success(PersonPage(juniors, tasks))
    } catch (e: CancellationException) {
        throw e
    } catch (e: OfflineException) {
        Result.failure(Exception("You're offline. The hierarchy needs a connection."))
    } catch (e: UnauthorizedException) {
        logout()
        _login.update { it.copy(error = "Your session expired. Log in again.") }
        Result.failure(e)
    } catch (e: Exception) {
        Result.failure(Exception(e.message ?: "Couldn't load the hierarchy"))
    }

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
                saveRole(me)
                _profile.value = currentProfile()
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

    /** A sync the user asked for (sync button, pull to refresh, a status change): always toasts. */
    fun syncNow() {
        viewModelScope.launch { runSync(userAsked = true) }
    }

    /** Logs out but keeps the local data, so queued changes are sent after the next login. */
    fun logout() {
        autoSync?.cancel()
        c.session.token = null
        _loggedIn.value = false
    }

    // Background syncs run every 30 s, so they only toast when they start failing,
    // not on every success or on every repeat of the same failure.
    private var backgroundFailing = false

    private suspend fun runSync(userAsked: Boolean) {
        val result = c.sync.sync()
        val failed = result is SyncStatus.Failed || result == SyncStatus.Offline
        when {
            result == SyncStatus.NeedsLogin -> {
                logout()
                _login.update { it.copy(error = "Your session expired. Log in again to sync.") }
            }
            failed && (userAsked || !backgroundFailing) -> _toasts.tryEmit(
                if (result == SyncStatus.Offline) "Server error: can't reach the server" else "Server error",
            )
            !failed && userAsked -> _toasts.tryEmit("Synced")
        }
        if (!userAsked) backgroundFailing = failed
    }

    private fun startAutoSync() {
        autoSync?.cancel()
        autoSync = viewModelScope.launch {
            while (true) {
                runSync(userAsked = false)
                delay(30_000)
            }
        }
    }
}
