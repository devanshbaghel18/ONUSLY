package com.onusly.app.data.model

import android.graphics.drawable.Drawable

enum class AppCategory(val title: String) {
    ALL("All"),
    SOCIAL("Social"),
    ENTERTAINMENT("Entertainment"),
    MESSAGING("Messaging"),
    GAMING("Gaming"),
    OTHER("Other")
}

data class InstalledAppInfo(
    val packageName: String,
    val label: String,
    val icon: Drawable? = null,
    val category: AppCategory = AppCategory.OTHER
)
