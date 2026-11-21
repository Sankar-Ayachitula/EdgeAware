package com.edgeaware.app.ui

import androidx.compose.ui.graphics.Color
import com.edgeaware.app.data.TaskStatus
import kotlin.time.Clock
import kotlin.time.ExperimentalTime

fun statusLabel(status: String) = when (status) {
    TaskStatus.TODO -> "To do"
    TaskStatus.IN_PROGRESS -> "In progress"
    TaskStatus.DONE -> "Done"
    else -> status
}

fun statusColor(status: String) = when (status) {
    TaskStatus.TODO -> Brand.New
    TaskStatus.IN_PROGRESS -> Brand.Ongoing
    TaskStatus.DONE -> Brand.Complete
    else -> Color.Gray
}

/** Card strip color: red once past the due date (unless done), otherwise the status color. */
fun accentColor(status: String, dueIso: String?) =
    if (isOverdue(dueIso, status)) Brand.Overdue else statusColor(status)

private val months = listOf("Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec")

/** "2026-11-21T10:00:00Z" -> "Nov 21, 2026" (date part only, as sent by the server). */
fun formatDate(iso: String?): String? {
    if (iso == null || iso.length < 10) return null
    val y = iso.substring(0, 4)
    val m = iso.substring(5, 7).toIntOrNull() ?: return null
    val d = iso.substring(8, 10).toIntOrNull() ?: return null
    return "${months.getOrNull(m - 1) ?: return null} $d, $y"
}

@OptIn(ExperimentalTime::class)
fun isOverdue(dueIso: String?, status: String): Boolean {
    if (dueIso == null || status == TaskStatus.DONE || dueIso.length < 10) return false
    val today = Clock.System.now().toString().take(10) // UTC date, yyyy-MM-dd
    return dueIso.take(10) < today
}

@OptIn(ExperimentalTime::class)
fun formatSyncedAgo(epochMillis: Long?): String {
    if (epochMillis == null) return "Not synced yet"
    val minutes = (Clock.System.now().toEpochMilliseconds() - epochMillis) / 60_000
    return when {
        minutes < 1 -> "Synced just now"
        minutes < 60 -> "Synced $minutes min ago"
        minutes < 24 * 60 -> "Synced ${minutes / 60} h ago"
        else -> "Synced ${minutes / (24 * 60)} days ago"
    }
}
