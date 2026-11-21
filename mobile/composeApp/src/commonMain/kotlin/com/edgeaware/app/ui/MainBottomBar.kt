package com.edgeaware.app.ui

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Assignment
import androidx.compose.material.icons.automirrored.outlined.Assignment
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.AccountTree
import androidx.compose.material.icons.outlined.AccountCircle
import androidx.compose.material.icons.outlined.AccountTree
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

/** The three bottom tabs. The selected tab shows a filled icon, the others outlined. */
enum class MainTab(val label: String, val selectedIcon: ImageVector, val icon: ImageVector) {
    Tasks("My tasks", Icons.AutoMirrored.Filled.Assignment, Icons.AutoMirrored.Outlined.Assignment),
    Hierarchy("Hierarchy", Icons.Filled.AccountTree, Icons.Outlined.AccountTree),
    Profile("Profile", Icons.Filled.AccountCircle, Icons.Outlined.AccountCircle),
}

@Composable
fun MainBottomBar(selected: MainTab, onSelect: (MainTab) -> Unit) {
    NavigationBar(
        containerColor = Color.White,
        tonalElevation = 0.dp,
        // A light shadow along the top edge separates the bar from the white content.
        modifier = Modifier.shadow(12.dp, ambientColor = Color.Black.copy(alpha = 0.15f), spotColor = Color.Black.copy(alpha = 0.25f)),
    ) {
        MainTab.entries.forEach { tab ->
            val isSelected = tab == selected
            NavigationBarItem(
                selected = isSelected,
                onClick = { onSelect(tab) },
                icon = { Icon(if (isSelected) tab.selectedIcon else tab.icon, contentDescription = tab.label) },
                label = {
                    Text(
                        tab.label,
                        style = MaterialTheme.typography.labelMedium,
                        fontWeight = if (isSelected) FontWeight.Bold else FontWeight.Medium,
                    )
                },
                colors = NavigationBarItemDefaults.colors(
                    indicatorColor = Brand.Yellow,
                    selectedIconColor = Brand.OnYellow,
                    selectedTextColor = Brand.OnYellow,
                    unselectedIconColor = Color(0xFF6B6455),
                    unselectedTextColor = Color(0xFF6B6455),
                ),
            )
        }
    }
}
