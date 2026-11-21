package com.edgeaware.app.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// JSON shapes of the EdgeAware backend API (see the backend README).

@Serializable
data class TaskDto(
    val id: String,
    val title: String,
    val description: String = "",
    val status: String,
    @SerialName("due_at") val dueAt: String? = null,
    @SerialName("completed_at") val completedAt: String? = null,
    @SerialName("assigned_to") val assignedTo: String,
    @SerialName("created_by") val createdBy: String,
    val version: Long,
    @SerialName("updated_at") val updatedAt: String = "",
)

@Serializable
data class RoleDto(val id: Long, val name: String, val level: Int)

@Serializable
data class UserDto(
    val id: String,
    val username: String,
    val role: RoleDto,
    @SerialName("manager_id") val managerId: String? = null,
)

@Serializable
data class LoginRequest(val username: String, val password: String)

@Serializable
data class LoginResponse(@SerialName("access_token") val accessToken: String)

@Serializable
data class ChangeDto(
    val seq: Long,
    val kind: String, // upsert | deleted | revoked
    @SerialName("task_id") val taskId: String,
    val task: TaskDto? = null,
)

@Serializable
data class PullResponse(
    val changes: List<ChangeDto>,
    @SerialName("next_cursor") val nextCursor: Long,
    @SerialName("has_more") val hasMore: Boolean,
)

@Serializable
data class SnapshotResponse(val tasks: List<TaskDto>, val cursor: Long)

@Serializable
data class StatusFields(val status: String)

@Serializable
data class PushChange(
    @SerialName("op_id") val opId: String,
    val op: String,
    @SerialName("task_id") val taskId: String,
    @SerialName("base_version") val baseVersion: Long,
    val fields: StatusFields,
)

@Serializable
data class PushRequest(
    @SerialName("device_id") val deviceId: String,
    @SerialName("device_name") val deviceName: String,
    val changes: List<PushChange>,
)

@Serializable
data class OpResult(
    @SerialName("op_id") val opId: String,
    val status: String, // applied | conflict | forbidden | not_found | invalid
    val error: String? = null,
    val task: TaskDto? = null,
    val duplicate: Boolean = false,
)

@Serializable
data class PushResponse(val results: List<OpResult>)

object TaskStatus {
    const val TODO = "todo"
    const val IN_PROGRESS = "in_progress"
    const val DONE = "done"
    val all = listOf(TODO, IN_PROGRESS, DONE)
}
