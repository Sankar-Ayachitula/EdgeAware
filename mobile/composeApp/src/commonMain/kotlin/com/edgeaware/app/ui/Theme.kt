package com.edgeaware.app.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.unit.dp

/** Yellow and white brand colors. */
object Brand {
    val Yellow = Color(0xFFFFD700)        // classic yellow: app bar, primary button
    val YellowLight = Color(0xFFFFF3C4)   // selected chips, highlights
    val OnYellow = Color(0xFF3B2F00)      // text and icons on yellow
    val Gold = Color(0xFF8A6A00)          // readable accent for text/icons on white
    val Background = Color.White          // app background; cards stand out by their shadow
    val CardWhite = Color.White

    // Status colors (cards' left strip and status pills).
    val New = Color(0xFF1E6FD9)       // to do: blue
    val Ongoing = Color(0xFFF08C00)   // in progress: orange
    val Complete = Color(0xFF2E8540)  // done: green
    val Overdue = Color(0xFFD32F2F)   // past due date: red
}

private val colors = lightColorScheme(
    primary = Brand.Gold,
    onPrimary = Color.White,
    primaryContainer = Brand.Yellow,
    onPrimaryContainer = Brand.OnYellow,
    secondaryContainer = Brand.YellowLight,
    onSecondaryContainer = Brand.OnYellow,
    background = Brand.Background,
    onBackground = Color(0xFF1F1B10),
    surface = Brand.Background,
    onSurface = Color(0xFF1F1B10),
    surfaceContainerLowest = Color.White,
    surfaceContainerLow = Color.White,
)

@Composable
fun EdgeAwareTheme(content: @Composable () -> Unit) = MaterialTheme(colorScheme = colors, content = content)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun yellowTopBarColors() = TopAppBarDefaults.topAppBarColors(
    containerColor = Brand.Yellow,
    scrolledContainerColor = Brand.Yellow,
    titleContentColor = Brand.OnYellow,
    navigationIconContentColor = Brand.OnYellow,
    actionIconContentColor = Brand.OnYellow,
)

/** A soft, wide shadow on all sides so a white card floats on the white background. */
fun Modifier.floatingCard(shape: Shape): Modifier = shadow(
    elevation = 10.dp,
    shape = shape,
    ambientColor = Color.Black.copy(alpha = 0.18f),
    spotColor = Color.Black.copy(alpha = 0.28f),
)

/** Two-letter initials for avatars: "operator_n1a" -> "ON", "director" -> "DI". */
fun initials(username: String): String {
    val parts = username.split('_', '.', '-', ' ').filter { it.isNotBlank() }
    return when {
        parts.size >= 2 -> "${parts[0][0]}${parts[1][0]}"
        else -> username.take(2)
    }.uppercase()
}
