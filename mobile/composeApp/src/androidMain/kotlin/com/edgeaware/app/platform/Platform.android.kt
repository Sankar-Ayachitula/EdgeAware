package com.edgeaware.app.platform

import android.content.Context
import android.os.Build
import app.cash.sqldelight.db.SqlDriver
import app.cash.sqldelight.driver.android.AndroidSqliteDriver
import com.edgeaware.app.db.EdgeAwareDb
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.engine.okhttp.OkHttp

actual val defaultServerUrl: String = "http://10.0.2.2:8080"

class AndroidPlatformDeps(private val context: Context) : PlatformDeps {
    override val keyValueStore: KeyValueStore = PrefsStore(context)
    override val deviceName: String = "${Build.MANUFACTURER} ${Build.MODEL}"
    override fun createSqlDriver(): SqlDriver = AndroidSqliteDriver(EdgeAwareDb.Schema, context, "edgeaware.db")
    override fun createHttpEngine(): HttpClientEngine = OkHttp.create()
}

// TODO: move the token to EncryptedSharedPreferences/Keystore before production.
private class PrefsStore(context: Context) : KeyValueStore {
    private val prefs = context.getSharedPreferences("edgeaware", Context.MODE_PRIVATE)
    override fun get(key: String): String? = prefs.getString(key, null)
    override fun put(key: String, value: String) = prefs.edit().putString(key, value).apply()
    override fun remove(key: String) = prefs.edit().remove(key).apply()
}
