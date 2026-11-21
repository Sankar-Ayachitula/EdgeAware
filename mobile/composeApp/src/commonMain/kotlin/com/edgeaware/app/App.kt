package com.edgeaware.app

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.graphics.Color
import androidx.lifecycle.viewmodel.compose.viewModel
import com.edgeaware.app.platform.PlatformBackHandler
import com.edgeaware.app.platform.PlatformDeps
import com.edgeaware.app.ui.AppViewModel
import com.edgeaware.app.ui.LoginScreen
import com.edgeaware.app.ui.TaskDetailScreen
import com.edgeaware.app.ui.TaskListScreen

private val colors = lightColorScheme(primary = Color(0xFF1F6FEB), secondary = Color(0xFF2E8540))

@Composable
fun App(deps: PlatformDeps) {
    val container = remember { AppContainer(deps) }
    val vm = viewModel { AppViewModel(container) }

    MaterialTheme(colorScheme = colors) {
        Surface {
            val loggedIn by vm.loggedIn.collectAsState()
            if (!loggedIn) {
                val login by vm.login.collectAsState()
                LoginScreen(login, onLogin = vm::login)
                return@Surface
            }

            val snackbar = remember { SnackbarHostState() }
            LaunchedEffect(Unit) { vm.notices.collect { snackbar.showSnackbar(it) } }

            var openTaskId by rememberSaveable { mutableStateOf<String?>(null) }
            PlatformBackHandler(enabled = openTaskId != null) { openTaskId = null }

            val id = openTaskId
            if (id != null) {
                val task by remember(id) { vm.task(id) }.collectAsState(null)
                TaskDetailScreen(task, snackbar, onBack = { openTaskId = null }, onSetStatus = { vm.setStatus(id, it) })
            } else {
                val tasks by vm.tasks.collectAsState()
                val status by vm.syncStatus.collectAsState()
                val pending by vm.pendingCount.collectAsState()
                val state by vm.syncState.collectAsState()
                TaskListScreen(
                    username = vm.username,
                    tasks = tasks,
                    syncStatus = status,
                    pendingCount = pending,
                    lastSyncAt = state?.last_sync_at,
                    snackbar = snackbar,
                    onOpen = { openTaskId = it },
                    onSync = vm::syncNow,
                    onLogout = vm::logout,
                )
            }
        }
    }
}
