package com.edgeaware.app.platform

import app.cash.sqldelight.db.SqlDriver
import app.cash.sqldelight.driver.native.NativeSqliteDriver
import com.edgeaware.app.db.EdgeAwareDb
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.engine.darwin.Darwin
import platform.Foundation.NSUserDefaults
import platform.UIKit.UIDevice

actual val defaultServerUrl: String = "http://localhost:8080"

class IosPlatformDeps : PlatformDeps {
    override val keyValueStore: KeyValueStore = DefaultsStore()
    override val deviceName: String = UIDevice.currentDevice.name
    override fun createSqlDriver(): SqlDriver = NativeSqliteDriver(EdgeAwareDb.Schema, "edgeaware.db")
    override fun createHttpEngine(): HttpClientEngine = Darwin.create()
}

// TODO: move the token to the Keychain before production.
private class DefaultsStore : KeyValueStore {
    private val defaults = NSUserDefaults.standardUserDefaults
    override fun get(key: String): String? = defaults.stringForKey(key)
    override fun put(key: String, value: String) = defaults.setObject(value, forKey = key)
    override fun remove(key: String) = defaults.removeObjectForKey(key)
}

@androidx.compose.runtime.Composable
actual fun PlatformBackHandler(enabled: Boolean, onBack: () -> Unit) {
}
