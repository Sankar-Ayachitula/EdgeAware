package com.edgeaware.app.data

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.bearerAuth
import io.ktor.client.request.get
import io.ktor.client.request.parameter
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.client.statement.HttpResponse
import io.ktor.client.statement.bodyAsText
import io.ktor.http.ContentType
import io.ktor.http.HttpStatusCode
import io.ktor.http.contentType
import io.ktor.serialization.kotlinx.json.json
import kotlinx.coroutines.CancellationException
import kotlinx.serialization.json.Json

/** The server rejected the token (expired or invalid): the user must log in again. */
class UnauthorizedException : Exception("Session expired, please log in again")

/** The server cleaned up the feed entries after our cursor: download a snapshot. */
class CursorExpiredException : Exception("Sync cursor expired")

/** The server could not be reached (no network, server down, timeout). */
class OfflineException(cause: Throwable) : Exception("Can't reach the server", cause)

/** Any other non-2xx response. */
class ApiException(val status: Int, message: String) : Exception(message)

class ApiClient(
    engine: HttpClientEngine,
    private val session: SessionStore,
) {
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

    private val http = HttpClient(engine) {
        expectSuccess = false
        install(ContentNegotiation) { json(json) }
        install(HttpTimeout) {
            connectTimeoutMillis = 10_000
            requestTimeoutMillis = 30_000
        }
    }

    private fun url(path: String) = session.serverUrl.trimEnd('/') + path

    private fun HttpRequestBuilder.auth() {
        session.token?.let { bearerAuth(it) }
    }

    suspend fun login(username: String, password: String): String {
        val resp = call {
            http.post(url("/login")) {
                contentType(ContentType.Application.Json)
                setBody(LoginRequest(username, password))
            }
        }
        if (resp.status == HttpStatusCode.Unauthorized) throw ApiException(401, "Wrong username or password")
        return resp.ok<LoginResponse>().accessToken
    }

    suspend fun me(): UserDto = call { http.get(url("/me")) { auth() } }.ok()

    suspend fun pull(since: Long, deviceId: String, limit: Int = 500): PullResponse {
        val resp = call {
            http.get(url("/sync/pull")) {
                auth()
                parameter("since", since)
                parameter("limit", limit)
                parameter("device_id", deviceId)
            }
        }
        if (resp.status == HttpStatusCode.Gone) throw CursorExpiredException()
        return resp.ok()
    }

    suspend fun snapshot(deviceId: String): SnapshotResponse =
        call { http.get(url("/sync/snapshot")) { auth(); parameter("device_id", deviceId) } }.ok()

    suspend fun push(request: PushRequest): PushResponse = call {
        http.post(url("/sync/push")) {
            auth()
            contentType(ContentType.Application.Json)
            setBody(request)
        }
    }.ok()

    private suspend fun call(block: suspend () -> HttpResponse): HttpResponse =
        try {
            block()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            throw OfflineException(e)
        }

    private suspend inline fun <reified T> HttpResponse.ok(): T {
        if (status == HttpStatusCode.Unauthorized) throw UnauthorizedException()
        if (status.value !in 200..299) {
            val msg = runCatching { bodyAsText() }.getOrDefault("")
            throw ApiException(status.value, "Server error ${status.value}: $msg")
        }
        return body()
    }
}
