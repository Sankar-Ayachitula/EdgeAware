package com.edgeaware.app

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.lifecycle.viewmodel.compose.viewModel
import com.edgeaware.app.platform.PlatformBackHandler
import com.edgeaware.app.platform.PlatformDeps
import com.edgeaware.app.ui.AppViewModel
import com.edgeaware.app.ui.EdgeAwareTheme
import com.edgeaware.app.ui.HierarchyScreen
import com.edgeaware.app.ui.LoginScreen
import com.edgeaware.app.ui.MainBottomBar
import com.edgeaware.app.ui.MainTab
import com.edgeaware.app.ui.PathEntry
import com.edgeaware.app.ui.ProfileScreen
import com.edgeaware.app.ui.TaskDetailScreen
import com.edgeaware.app.ui.TaskListScreen

@Composable
fun App(deps: PlatformDeps) {
    val container = remember { AppContainer(deps) }
    val vm = viewModel { AppViewModel(container) }

    EdgeAwareTheme {
        Surface {
            val loggedIn by vm.loggedIn.collectAsState()
            if (!loggedIn) {
                val login by vm.login.collectAsState()
                LoginScreen(login, onLogin = vm::login)
                return@Surface
            }

            val snackbar = remember { SnackbarHostState() }
            LaunchedEffect(Unit) { vm.notices.collect { snackbar.showSnackbar(it) } }

            val status by vm.syncStatus.collectAsState()
            val pending by vm.pendingCount.collectAsState()
            val state by vm.syncState.collectAsState()

            // A task opened from My tasks covers the whole screen (no bottom bar).
            var openTaskId by rememberSaveable { mutableStateOf<String?>(null) }
            val openId = openTaskId
            if (openId != null) {
                PlatformBackHandler(enabled = true) { openTaskId = null }
                val task by remember(openId) { vm.task(openId) }.collectAsState(null)
                TaskDetailScreen(task, snackbar, onBack = { openTaskId = null }, onSetStatus = { vm.setStatus(openId, it) })
                return@Surface
            }

            var tab by rememberSaveable { mutableStateOf(MainTab.Tasks) }
            // Where you are in the hierarchy tab; kept while switching tabs. Starts at you.
            val path = remember(vm.myUserId) { mutableStateListOf(PathEntry(vm.myUserId, vm.username, null)) }
            // Back on another tab returns to My tasks first.
            PlatformBackHandler(enabled = tab != MainTab.Tasks && !(tab == MainTab.Hierarchy && path.size > 1)) { tab = MainTab.Tasks }

            Scaffold(
                bottomBar = { MainBottomBar(tab, onSelect = { tab = it }) },
                // Each tab draws its own top bar under the status bar.
                contentWindowInsets = WindowInsets(0),
            ) { padding ->
                Box(Modifier.padding(padding).consumeWindowInsets(padding)) {
                    when (tab) {
                        MainTab.Tasks -> {
                            val tasks by vm.tasks.collectAsState()
                            TaskListScreen(
                                username = vm.username,
                                tasks = tasks,
                                syncStatus = status,
                                pendingCount = pending,
                                lastSyncAt = state?.last_sync_at,
                                snackbar = snackbar,
                                onOpen = { openTaskId = it },
                                onSync = vm::syncNow,
                            )
                        }
                        MainTab.Hierarchy -> HierarchyScreen(
                            path = path,
                            onOpen = { path += PathEntry(it.id, it.username, it.role.name) },
                            onBack = { if (path.size > 1) path.removeAt(path.lastIndex) },
                            load = vm::loadPerson,
                        )
                        MainTab.Profile -> {
                            val profile by vm.profile.collectAsState()
                            ProfileScreen(
                                profile = profile,
                                syncStatus = status,
                                pendingCount = pending,
                                lastSyncAt = state?.last_sync_at,
                                onRefresh = vm::refreshProfile,
                                onSync = vm::syncNow,
                                onLogout = vm::logout,
                            )
                        }
                    }
                }
            }
        }
    }
}
