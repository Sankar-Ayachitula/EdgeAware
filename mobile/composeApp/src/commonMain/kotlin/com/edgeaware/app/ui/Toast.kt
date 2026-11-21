package com.edgeaware.app.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.collectLatest

/**
 * A small toast near the bottom of the screen, drawn on top of everything (shared code,
 * so it looks the same on Android and iOS). Each new message replaces the current one.
 */
@Composable
fun ToastHost(messages: Flow<String>, bottomPadding: Int = 104) {
    var message by remember { mutableStateOf<String?>(null) }
    var visible by remember { mutableStateOf(false) }
    LaunchedEffect(messages) {
        messages.collectLatest {
            message = it
            visible = true
            delay(2_000)
            visible = false
        }
    }

    Box(Modifier.fillMaxSize().padding(bottom = bottomPadding.dp, start = 24.dp, end = 24.dp), contentAlignment = Alignment.BottomCenter) {
        AnimatedVisibility(
            visible = visible,
            enter = fadeIn() + slideInVertically { it / 2 },
            exit = fadeOut() + slideOutVertically { it / 2 },
        ) {
            val text = message ?: ""
            val isError = text.startsWith("Server error")
            Surface(shape = RoundedCornerShape(50), color = Color(0xF2222222), shadowElevation = 6.dp) {
                Row(Modifier.padding(horizontal = 18.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        if (isError) Icons.Filled.ErrorOutline else Icons.Filled.CheckCircle,
                        null,
                        tint = if (isError) Color(0xFFFF8A80) else Color(0xFF81C995),
                        modifier = Modifier.size(18.dp),
                    )
                    Spacer(Modifier.width(10.dp))
                    Text(text, color = Color.White, style = MaterialTheme.typography.bodyMedium)
                }
            }
        }
    }
}
