package com.edgeaware.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Event
import androidx.compose.material.icons.outlined.AccountTree
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.edgeaware.app.data.TaskDto
import com.edgeaware.app.data.UserDto
import com.edgeaware.app.platform.PlatformBackHandler

/** Someone on the path from you down to the person being viewed. */
data class PathEntry(val id: String, val username: String, val roleName: String?)

/**
 * The hierarchy tab: starts at you and lists who reports to you. Tapping someone opens
 * their page (their team and their tasks); you can keep going down. You never see
 * anyone above you. [path] is owned by the caller so it survives switching tabs.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HierarchyScreen(
    path: List<PathEntry>,
    onOpen: (UserDto) -> Unit,
    onBack: () -> Unit,
    load: suspend (String) -> Result<PersonPage>,
) {
    val current = path.last()
    val isMe = path.size == 1
    var page by remember(current.id) { mutableStateOf<PersonPage?>(null) }
    var error by remember(current.id) { mutableStateOf<String?>(null) }
    var loading by remember(current.id) { mutableStateOf(true) }
    var reload by remember(current.id) { mutableIntStateOf(0) }

    LaunchedEffect(current.id, reload) {
        loading = true
        load(current.id).onSuccess { page = it; error = null }.onFailure { error = it.message }
        loading = false
    }

    PlatformBackHandler(enabled = !isMe, onBack = onBack)

    Scaffold(
        topBar = {
            TopAppBar(
                colors = yellowTopBarColors(),
                navigationIcon = {
                    if (!isMe) IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back") }
                },
                title = {
                    Column {
                        Text(if (isMe) "Hierarchy" else current.username, fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(
                            if (isMe) "People who report to you" else (current.roleName ?: ""),
                            style = MaterialTheme.typography.bodySmall,
                            color = Brand.OnYellow.copy(alpha = 0.75f),
                        )
                    }
                },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize()) {
            if (path.size > 2) Breadcrumb(path)
            PullToRefreshBox(
                isRefreshing = loading && page != null,
                onRefresh = { reload++ },
                modifier = Modifier.fillMaxSize(),
            ) {
                val p = page
                when {
                    p == null && loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator(color = Brand.Gold) }
                    p == null -> Message(Icons.Outlined.CloudOff, "Couldn't load", error ?: "", action = "Try again") { reload++ }
                    else -> PersonContent(p, isMe, current.username, onOpen)
                }
            }
        }
    }
}

@Composable
private fun PersonContent(page: PersonPage, isMe: Boolean, name: String, onOpen: (UserDto) -> Unit) {
    if (isMe && page.juniors.isEmpty()) {
        Message(Icons.Outlined.AccountTree, "No one reports to you", "When people are added to your team, they'll show up here.")
        return
    }
    LazyColumn(
        contentPadding = PaddingValues(horizontal = 18.dp, vertical = 16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
        modifier = Modifier.fillMaxSize(),
    ) {
        if (page.juniors.isNotEmpty()) {
            item { SectionHeader(if (isMe) "Your team · ${page.juniors.size}" else "Reports to $name · ${page.juniors.size}") }
            items(page.juniors, key = { "u" + it.id }) { PersonCard(it, onClick = { onOpen(it) }) }
        }
        if (!isMe) {
            item { SectionHeader("Tasks · ${page.tasks.size}", top = page.juniors.isNotEmpty()) }
            if (page.tasks.isEmpty()) {
                item { Text("No tasks assigned.", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(vertical = 8.dp)) }
            } else {
                val sorted = page.tasks.sortedWith(
                    compareBy<TaskDto>({ statusOrder(it.status) }, { it.dueAt == null }, { it.dueAt }, { it.title }),
                )
                items(sorted, key = { "t" + it.id }) { JuniorTaskCard(it) }
            }
        }
    }
}

private fun statusOrder(status: String) = when (status) {
    "in_progress" -> 0
    "todo" -> 1
    else -> 2
}

@Composable
private fun SectionHeader(text: String, top: Boolean = false) {
    Text(
        text,
        style = MaterialTheme.typography.titleSmall,
        fontWeight = FontWeight.Bold,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(top = if (top) 10.dp else 0.dp),
    )
}

@Composable
private fun Breadcrumb(path: List<PathEntry>) {
    Text(
        path.joinToString("  ›  ") { if (it == path.first()) "You" else it.username },
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 8.dp),
    )
}

@Composable
fun Avatar(username: String, size: Int = 44) {
    Box(Modifier.size(size.dp).background(Brand.Yellow, CircleShape), contentAlignment = Alignment.Center) {
        Text(
            initials(username),
            color = Brand.OnYellow,
            fontWeight = FontWeight.Bold,
            style = if (size >= 64) MaterialTheme.typography.headlineSmall else MaterialTheme.typography.titleSmall,
        )
    }
}

@Composable
fun RoleChip(name: String, level: Int?) {
    Surface(shape = RoundedCornerShape(50), color = Brand.YellowLight) {
        Text(
            if (level != null) "$name · Level $level" else name,
            style = MaterialTheme.typography.labelMedium,
            color = Brand.OnYellow,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
        )
    }
}

@Composable
private fun PersonCard(u: UserDto, onClick: () -> Unit) {
    val shape = RoundedCornerShape(14.dp)
    Card(
        onClick = onClick,
        shape = shape,
        colors = CardDefaults.cardColors(containerColor = Brand.CardWhite),
        modifier = Modifier.fillMaxWidth().floatingCard(shape),
    ) {
        Row(Modifier.padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
            Avatar(u.username)
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(u.username, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                RoleChip(u.role.name, u.role.level)
            }
            Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = "Open", tint = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/** A junior's task, read-only, in the same style as the My tasks cards. */
@Composable
private fun JuniorTaskCard(t: TaskDto) {
    val overdue = isOverdue(t.dueAt, t.status)
    val shape = RoundedCornerShape(14.dp)
    Card(
        shape = shape,
        colors = CardDefaults.cardColors(containerColor = Brand.CardWhite),
        modifier = Modifier.fillMaxWidth().floatingCard(shape),
    ) {
        Row(Modifier.height(IntrinsicSize.Min)) {
            Box(Modifier.width(6.dp).fillMaxHeight().background(accentColor(t.status, t.dueAt)))
            Column(Modifier.weight(1f).padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(t.title, style = MaterialTheme.typography.titleMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    StatusPill(t.status)
                    formatDate(t.dueAt)?.let { due ->
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

/** Centered icon + message; a LazyColumn so pull-to-refresh still works. */
@Composable
private fun Message(icon: ImageVector, title: String, body: String, action: String? = null, onAction: () -> Unit = {}) {
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(32.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        item {
            Spacer(Modifier.height(64.dp))
            Box(Modifier.size(88.dp).background(Brand.YellowLight, CircleShape), contentAlignment = Alignment.Center) {
                Icon(icon, null, tint = Brand.Gold, modifier = Modifier.size(44.dp))
            }
            Spacer(Modifier.height(20.dp))
            Text(title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, textAlign = TextAlign.Center)
            Spacer(Modifier.height(8.dp))
            Text(body, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
            if (action != null) {
                Spacer(Modifier.height(20.dp))
                Button(onClick = onAction, colors = ButtonDefaults.buttonColors(containerColor = Brand.Yellow, contentColor = Brand.OnYellow)) { Text(action) }
            }
        }
    }
}
