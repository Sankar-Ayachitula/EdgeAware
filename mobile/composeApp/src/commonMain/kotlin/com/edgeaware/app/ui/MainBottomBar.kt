package com.edgeaware.app.ui

import androidx.compose.animation.animateColorAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Assignment
import androidx.compose.material.icons.automirrored.outlined.Assignment
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.AccountTree
import androidx.compose.material.icons.outlined.AccountCircle
import androidx.compose.material.icons.outlined.AccountTree
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

/** The three bottom tabs. The selected tab shows a filled icon, the others outlined. */
enum class MainTab(val label: String, val selectedIcon: ImageVector, val icon: ImageVector) {
    Tasks("My tasks", Icons.AutoMirrored.Filled.Assignment, Icons.AutoMirrored.Outlined.Assignment),
    Hierarchy("Hierarchy", Icons.Filled.AccountTree, Icons.Outlined.AccountTree),
    Profile("Profile", Icons.Filled.AccountCircle, Icons.Outlined.AccountCircle),
}

private val Unselected = Color(0xFF6B6455)

/**
 * Bottom navigation. Drawn by hand instead of Material's NavigationBar because its
 * selected-tab pill has a fixed 64x32 size; ours is bigger.
 */
@Composable
fun MainBottomBar(selected: MainTab, onSelect: (MainTab) -> Unit) {
    Surface(
        color = Color.White,
        // A light shadow along the top edge separates the bar from the white content.
        modifier = Modifier.shadow(12.dp, ambientColor = Color.Black.copy(alpha = 0.15f), spotColor = Color.Black.copy(alpha = 0.25f)),
    ) {
        Row(
            Modifier.fillMaxWidth().navigationBarsPadding().padding(top = 10.dp, bottom = 8.dp).selectableGroup(),
            horizontalArrangement = Arrangement.SpaceEvenly,
        ) {
            MainTab.entries.forEach { tab -> TabItem(tab, tab == selected) { onSelect(tab) } }
        }
    }
}

@Composable
private fun RowScope.TabItem(tab: MainTab, selected: Boolean, onClick: () -> Unit) {
    val pill by animateColorAsState(if (selected) Brand.Yellow else Color.Transparent)
    val content = if (selected) Brand.OnYellow else Unselected
    Column(
        Modifier.weight(1f).selectable(
            selected = selected,
            onClick = onClick,
            role = Role.Tab,
            interactionSource = remember { MutableInteractionSource() },
            indication = null,
        ),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Box(
            Modifier.size(width = 80.dp, height = 44.dp).background(pill, RoundedCornerShape(50)),
            contentAlignment = Alignment.Center,
        ) {
            Icon(if (selected) tab.selectedIcon else tab.icon, contentDescription = tab.label, tint = content, modifier = Modifier.size(26.dp))
        }
        Text(
            tab.label,
            style = MaterialTheme.typography.labelMedium,
            fontWeight = if (selected) FontWeight.Bold else FontWeight.Medium,
            color = content,
            modifier = Modifier.padding(top = 6.dp),
        )
    }
}
