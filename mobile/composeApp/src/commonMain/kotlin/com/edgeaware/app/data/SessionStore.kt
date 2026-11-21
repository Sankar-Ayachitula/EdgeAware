package com.edgeaware.app.data

import com.edgeaware.app.platform.KeyValueStore
import com.edgeaware.app.platform.defaultServerUrl
import kotlin.uuid.ExperimentalUuidApi
import kotlin.uuid.Uuid

/**
 * Small persistent settings: server address, login token, who is logged in, and
 * this install's device ID (generated once, used by the server for sync).
 */
class SessionStore(private val store: KeyValueStore) {

    var serverUrl: String
        get() = store.get(KEY_SERVER) ?: defaultServerUrl
        set(value) = store.put(KEY_SERVER, value.trim())

    var token: String?
        get() = store.get(KEY_TOKEN)
        set(value) = if (value == null) store.remove(KEY_TOKEN) else store.put(KEY_TOKEN, value)

    var userId: String?
        get() = store.get(KEY_USER_ID)
        set(value) = if (value == null) store.remove(KEY_USER_ID) else store.put(KEY_USER_ID, value)

    var username: String?
        get() = store.get(KEY_USERNAME)
        set(value) = if (value == null) store.remove(KEY_USERNAME) else store.put(KEY_USERNAME, value)

    /** The logged-in user's role, saved at login so the profile shows it offline. */
    var roleName: String?
        get() = store.get(KEY_ROLE_NAME)
        set(value) = if (value == null) store.remove(KEY_ROLE_NAME) else store.put(KEY_ROLE_NAME, value)

    var roleLevel: Int?
        get() = store.get(KEY_ROLE_LEVEL)?.toIntOrNull()
        set(value) = if (value == null) store.remove(KEY_ROLE_LEVEL) else store.put(KEY_ROLE_LEVEL, value.toString())

    @OptIn(ExperimentalUuidApi::class)
    val deviceId: String
        get() = store.get(KEY_DEVICE) ?: Uuid.random().toString().also { store.put(KEY_DEVICE, it) }

    val isLoggedIn: Boolean get() = token != null && userId != null

    private companion object {
        const val KEY_SERVER = "server_url"
        const val KEY_TOKEN = "token"
        const val KEY_USER_ID = "user_id"
        const val KEY_USERNAME = "username"
        const val KEY_DEVICE = "device_id"
        const val KEY_ROLE_NAME = "role_name"
        const val KEY_ROLE_LEVEL = "role_level"
    }
}
