package com.edgeaware.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Logout
import androidx.compose.material.icons.outlined.CloudUpload
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.PhoneAndroid
import androidx.compose.material.icons.outlined.Sync
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.edgeaware.app.data.SyncStatus

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProfileScreen(
    profile: ProfileUiState,
    syncStatus: SyncStatus,
    pendingCount: Long,
    lastSyncAt: Long?,
    onRefresh: () -> Unit,
    onSync: () -> Unit,
    onLogout: () -> Unit,
) {
    var confirmLogout by remember { mutableStateOf(false) }
    LaunchedEffect(Unit) { onRefresh() }

    Scaffold(
        topBar = { TopAppBar(colors = yellowTopBarColors(), title = { Text("Profile", fontWeight = FontWeight.Bold) }) },
    ) { padding ->
        Column(
            Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 18.dp, vertical = 24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Avatar(profile.username, size = 88)
            Spacer(Modifier.height(14.dp))
            Text(profile.username, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
            profile.roleName?.let {
                Spacer(Modifier.height(8.dp))
                RoleChip(it, profile.roleLevel)
            }
            Spacer(Modifier.height(28.dp))

            InfoCard("Sync") {
                val syncText = when (syncStatus) {
                    SyncStatus.Syncing -> "Syncing…"
                    SyncStatus.Offline -> "Offline · ${formatSyncedAgo(lastSyncAt).lowercase()}"
                    is SyncStatus.Failed -> "Problem: ${syncStatus.message}"
                    else -> formatSyncedAgo(lastSyncAt)
                }
                InfoRow(Icons.Outlined.Sync, "Last sync", syncText)
                HorizontalDivider(Modifier.padding(start = 52.dp))
                InfoRow(Icons.Outlined.CloudUpload, "Waiting to sync", if (pendingCount == 0L) "Nothing" else "$pendingCount change(s)")
                TextButton(
                    onClick = onSync,
                    enabled = syncStatus != SyncStatus.Syncing,
                    modifier = Modifier.align(Alignment.End).padding(end = 8.dp, bottom = 4.dp),
                ) { Text("Sync now", color = Brand.Gold, fontWeight = FontWeight.Bold) }
            }
            Spacer(Modifier.height(18.dp))
            InfoCard("This device") {
                InfoRow(Icons.Outlined.PhoneAndroid, "Device", profile.deviceName)
                HorizontalDivider(Modifier.padding(start = 52.dp))
                InfoRow(Icons.Outlined.Dns, "Server", profile.serverUrl)
            }
            Spacer(Modifier.height(28.dp))
            OutlinedButton(
                onClick = { if (pendingCount > 0) confirmLogout = true else onLogout() },
                colors = ButtonDefaults.outlinedButtonColors(contentColor = Brand.Overdue),
                modifier = Modifier.fillMaxWidth().height(50.dp),
            ) {
                Icon(Icons.AutoMirrored.Filled.Logout, null, Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text("Log out", fontWeight = FontWeight.Bold)
            }
        }
    }

    if (confirmLogout) {
        AlertDialog(
            onDismissRequest = { confirmLogout = false },
            title = { Text("Changes not sent yet") },
            text = { Text("$pendingCount change(s) haven't reached the server. They stay on this phone and are sent the next time you log in.") },
            confirmButton = { TextButton(onClick = { confirmLogout = false; onLogout() }) { Text("Log out anyway") } },
            dismissButton = { TextButton(onClick = { confirmLogout = false }) { Text("Stay") } },
        )
    }
}

@Composable
private fun InfoCard(title: String, content: @Composable androidx.compose.foundation.layout.ColumnScope.() -> Unit) {
    val shape = RoundedCornerShape(14.dp)
    Column(Modifier.fillMaxWidth()) {
        Text(
            title,
            style = MaterialTheme.typography.titleSmall,
            fontWeight = FontWeight.Bold,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(start = 4.dp, bottom = 8.dp),
        )
        Card(
            shape = shape,
            colors = CardDefaults.cardColors(containerColor = Brand.CardWhite),
            modifier = Modifier.fillMaxWidth().floatingCard(shape),
        ) { Column(Modifier.padding(vertical = 4.dp), content = content) }
    }
}

@Composable
private fun InfoRow(icon: ImageVector, label: String, value: String) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, null, tint = Brand.Gold, modifier = Modifier.size(22.dp))
        Spacer(Modifier.width(14.dp))
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text(value, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}
