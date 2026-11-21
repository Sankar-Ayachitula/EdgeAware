package com.edgeaware.app

import androidx.compose.ui.window.ComposeUIViewController
import com.edgeaware.app.platform.IosPlatformDeps

/** Called from Swift (iosApp/iosApp/ContentView.swift) to host the shared Compose UI. */
fun MainViewController() = ComposeUIViewController { App(IosPlatformDeps()) }
