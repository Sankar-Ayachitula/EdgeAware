package com.edgeaware.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.CloudUpload
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.edgeaware.app.data.TaskStatus
import com.edgeaware.app.db.VisibleTask

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TaskDetailScreen(
    task: VisibleTask?,
    snackbar: SnackbarHostState,
    onBack: () -> Unit,
    onSetStatus: (String) -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                colors = yellowTopBarColors(),
                title = { Text("Task") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back") } },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        if (task == null) {
            // Deleted or reassigned while open.
            Text("This task is no longer available.", Modifier.padding(padding).padding(24.dp))
            return@Scaffold
        }
        Column(
            Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(task.title, style = MaterialTheme.typography.headlineSmall)
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusPill(task.status)
                if (task.pending) {
                    Spacer(Modifier.width(12.dp))
                    Icon(Icons.Filled.CloudUpload, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(16.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("Waiting to sync", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.primary)
                }
            }

            Text("Update status", style = MaterialTheme.typography.titleSmall)
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                TaskStatus.all.forEachIndexed { i, s ->
                    SegmentedButton(
                        selected = task.status == s,
                        onClick = { if (task.status != s) onSetStatus(s) },
                        shape = SegmentedButtonDefaults.itemShape(i, TaskStatus.all.size),
                    ) { Text(statusLabel(s)) }
                }
            }

            HorizontalDivider()
            Detail("Due", formatDate(task.due_at)?.let { if (isOverdue(task.due_at, task.status)) "$it (overdue)" else it } ?: "No due date")
            formatDate(task.completed_at)?.let { Detail("Completed", it) }
            Detail("Description", task.description.ifBlank { "No description" })
        }
    }
}

@Composable
private fun Detail(label: String, value: String) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(value, style = MaterialTheme.typography.bodyLarge)
    }
}
