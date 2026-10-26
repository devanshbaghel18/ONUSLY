package com.onusly.app.engine

import android.content.Context
import android.content.Intent
import com.onusly.app.data.model.InstalledAppInfo

object InstalledAppsManager {

    val SYSTEM_PLATFORM_PREFIXES = listOf(
        "android",
        "com.android.",
        "com.google.android.",
        "com.samsung.android.",
        "com.sec.android.",
        "com.miui.",
        "com.oppo.",
        "com.coloros.",
        "com.oplus.",
        "com.vivo.",
        "com.bbk.",
        "com.huawei.",
        "com.motorola.",
        "com.lge."
    )

    fun isSystemOrExcluded(
        packageName: String,
        allowedPackages: Set<String> = BlockerEngine.ALWAYS_ALLOWED_PACKAGES
    ): Boolean {
        val clean = packageName.trim()
        if (clean.isBlank()) return true
        val lower = clean.lowercase()
        if (lower == "android") return true
        if (allowedPackages.any { it.equals(clean, ignoreCase = true) }) return true

        val hasSystemPrefix = SYSTEM_PLATFORM_PREFIXES.any { prefix ->
            if (prefix == "android") {
                lower == "android" || lower.startsWith("android.")
            } else {
                lower.startsWith(prefix)
            }
        }

        if (!hasSystemPrefix) {
            return false
        }

        return lower.contains("settings") ||
                lower.contains("launcher") ||
                lower.contains("systemui") ||
                lower.contains("dialer") ||
                lower.contains("telecom") ||
                lower.contains("emergency") ||
                lower.contains("incallui") ||
                lower.contains("cellbroadcast") ||
                lower.contains("safetyhub")
    }

    fun getInstalledApps(context: Context): List<InstalledAppInfo> {
        val pm = context.packageManager
        val intent = Intent(Intent.ACTION_MAIN, null).apply {
            addCategory(Intent.CATEGORY_LAUNCHER)
        }

        val resolveInfos = try {
            pm.queryIntentActivities(intent, 0)
        } catch (_: Exception) {
            emptyList()
        }

        val seenPackages = mutableSetOf<String>()
        val apps = mutableListOf<InstalledAppInfo>()

        for (info in resolveInfos) {
            val pkg = info.activityInfo?.packageName ?: continue
            if (pkg == context.packageName) continue
            if (seenPackages.contains(pkg)) continue
            if (isSystemOrExcluded(pkg)) continue

            seenPackages.add(pkg)
            val label = try {
                info.loadLabel(pm)?.toString()?.trim() ?: pkg
            } catch (_: Exception) {
                pkg
            }
            val icon = try {
                info.loadIcon(pm)
            } catch (_: Exception) {
                null
            }

            apps.add(InstalledAppInfo(packageName = pkg, label = label, icon = icon))
        }

        return apps.sortedBy { it.label.lowercase() }
    }

    fun filterApps(
        apps: List<InstalledAppInfo>,
        query: String?
    ): List<InstalledAppInfo> {
        if (query.isNullOrBlank()) return apps
        val cleanQuery = query.trim().lowercase()
        return apps.filter {
            it.label.lowercase().contains(cleanQuery) ||
            it.packageName.lowercase().contains(cleanQuery)
        }
    }
}
