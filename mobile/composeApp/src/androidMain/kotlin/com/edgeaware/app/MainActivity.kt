package com.edgeaware.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.edgeaware.app.platform.AndroidPlatformDeps

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        val deps = AndroidPlatformDeps(applicationContext)
        setContent { App(deps) }
    }
}
