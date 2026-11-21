package com.edgeaware.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CloudOff
import androidx.compose.material.icons.filled.CloudUpload
import androidx.compose.material.icons.filled.Event
import androidx.compose.material.icons.filled.Sync
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.edgeaware.app.data.SyncStatus
import com.edgeaware.app.data.TaskStatus
import com.edgeaware.app.db.VisibleTasks

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TaskListScreen(
    username: String,
    tasks: List<VisibleTasks>,
    syncStatus: SyncStatus,
    pendingCount: Long,
    lastSyncAt: Long?,
    snackbar: SnackbarHostState,
    onOpen: (String) -> Unit,
    onSync: () -> Unit,
) {
    var filter by rememberSaveable { mutableStateOf<String?>(null) }
    val shown = if (filter == null) tasks else tasks.filter { it.status == filter }

    Scaffold(
        topBar = {
            TopAppBar(
                colors = yellowTopBarColors(),
                title = {
                    Column {
                        Text("My tasks", fontWeight = FontWeight.Bold)
                        Text(username, style = MaterialTheme.typography.bodySmall, color = Brand.OnYellow.copy(alpha = 0.75f))
                    }
                },
                actions = {
                    IconButton(onClick = onSync, enabled = syncStatus != SyncStatus.Syncing) {
                        if (syncStatus == SyncStatus.Syncing) CircularProgressIndicator(Modifier.size(20.dp), color = Brand.OnYellow, strokeWidth = 2.dp)
                        else Icon(Icons.Filled.Sync, contentDescription = "Sync now")
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize()) {
            SyncBanner(syncStatus, pendingCount, lastSyncAt)
            LazyRow(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                item { FilterChip(selected = filter == null, onClick = { filter = null }, label = { Text("All (${tasks.size})") }) }
                items(TaskStatus.all) { s ->
                    val n = tasks.count { it.status == s }
                    FilterChip(selected = filter == s, onClick = { filter = s }, label = { Text("${statusLabel(s)} ($n)") })
                }
            }
            PullToRefreshBox(isRefreshing = syncStatus == SyncStatus.Syncing, onRefresh = onSync, modifier = Modifier.fillMaxSize()) {
                if (shown.isEmpty()) {
                    EmptyState(if (tasks.isEmpty()) "No tasks assigned to you yet." else "No tasks in this list.")
                } else {
                    LazyColumn(contentPadding = PaddingValues(horizontal = 18.dp, vertical = 16.dp), verticalArrangement = Arrangement.spacedBy(16.dp), modifier = Modifier.fillMaxSize()) {
                        items(shown, key = { it.id }) { TaskCard(it, onClick = { onOpen(it.id) }) }
                    }
                }
            }
        }
    }
}

@Composable
private fun SyncBanner(status: SyncStatus, pending: Long, lastSyncAt: Long?) {
    val (icon, text, color) = when {
        status == SyncStatus.Offline -> Triple(
            Icons.Filled.CloudOff,
            if (pending > 0) "Offline · $pending change(s) will sync when you're back online" else "Offline · showing saved tasks",
            Color(0xFF8A5A00),
        )
        status is SyncStatus.Failed -> Triple(Icons.Filled.CloudOff, "Sync problem: ${status.message}", MaterialTheme.colorScheme.error)
        pending > 0 -> Triple(Icons.Filled.CloudUpload, "$pending change(s) waiting to sync", MaterialTheme.colorScheme.primary)
        else -> Triple(Icons.Filled.Sync, formatSyncedAgo(lastSyncAt), MaterialTheme.colorScheme.onSurfaceVariant)
    }
    Row(
        Modifier.fillMaxWidth().background(color.copy(alpha = 0.10f)).padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, null, tint = color, modifier = Modifier.size(16.dp))
        Spacer(Modifier.width(8.dp))
        Text(text, style = MaterialTheme.typography.bodySmall, color = color)
    }
}

@Composable
private fun TaskCard(t: VisibleTasks, onClick: () -> Unit) {
    val overdue = isOverdue(t.due_at, t.status)
    val shape = RoundedCornerShape(14.dp)
    Card(
        onClick = onClick,
        shape = shape,
        colors = CardDefaults.cardColors(containerColor = Brand.CardWhite),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp),
        modifier = Modifier.fillMaxWidth().floatingCard(shape),
    ) {
        // IntrinsicSize.Min lets the strip stretch to the card's full height.
        Row(Modifier.height(IntrinsicSize.Min)) {
            Box(Modifier.width(6.dp).fillMaxHeight().background(accentColor(t.status, t.due_at)))
            Column(Modifier.weight(1f).padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(t.title, style = MaterialTheme.typography.titleMedium, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                    if (t.pending) Icon(Icons.Filled.CloudUpload, contentDescription = "Waiting to sync", tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(18.dp))
                }
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    StatusPill(t.status)
                    formatDate(t.due_at)?.let { due ->
                        val dateColor = if (overdue) Brand.Overdue else MaterialTheme.colorScheme.onSurfaceVariant
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Icon(Icons.Filled.Event, null, Modifier.size(14.dp), tint = dateColor)
                            Spacer(Modifier.width(4.dp))
                            Text(if (overdue) "Overdue · $due" else "Due $due", style = MaterialTheme.typography.bodySmall, color = dateColor)
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun StatusPill(status: String) {
    val color = statusColor(status)
    Surface(shape = RoundedCornerShape(50), color = color.copy(alpha = 0.12f)) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(8.dp).background(color, CircleShape))
            Spacer(Modifier.width(6.dp))
            Text(statusLabel(status), style = MaterialTheme.typography.labelMedium, color = color)
        }
    }
}

@Composable
internal fun EmptyState(message: String) {
    // LazyColumn so pull-to-refresh still works on an empty list.
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(32.dp)) {
        item {
            Text(message, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.fillMaxWidth().padding(top = 48.dp))
        }
    }
}
