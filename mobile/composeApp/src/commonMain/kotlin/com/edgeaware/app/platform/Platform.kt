package com.edgeaware.app.platform

import app.cash.sqldelight.db.SqlDriver
import io.ktor.client.engine.HttpClientEngine

/** Persistent string settings (SharedPreferences on Android, NSUserDefaults on iOS). */
interface KeyValueStore {
    fun get(key: String): String?
    fun put(key: String, value: String)
    fun remove(key: String)
}

/** Everything the shared code needs from the platform. */
interface PlatformDeps {
    val keyValueStore: KeyValueStore
    val deviceName: String
    fun createSqlDriver(): SqlDriver
    fun createHttpEngine(): HttpClientEngine
}

/**
 * Where the dev backend is reachable from the emulator/simulator: the Android
 * emulator sees the host Mac as 10.0.2.2, the iOS simulator as localhost.
 */
expect val defaultServerUrl: String

/** System back button/gesture (Android); iOS has no system back, so it does nothing there. */
@androidx.compose.runtime.Composable
expect fun PlatformBackHandler(enabled: Boolean, onBack: () -> Unit)
