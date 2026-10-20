package com.onusly.app.data.model

import android.graphics.drawable.Drawable

data class InstalledAppInfo(
    val packageName: String,
    val label: String,
    val icon: Drawable? = null
)
