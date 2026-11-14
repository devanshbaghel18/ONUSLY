package com.onusly.app.engine

import android.content.Context
import android.content.Intent
import android.content.pm.ApplicationInfo
import com.onusly.app.data.model.AppCategory
import com.onusly.app.data.model.InstalledAppInfo
import com.onusly.app.data.model.TargetCatalog

object InstalledAppsManager {

    private val PACKAGE_NAME_REGEX = Regex("^[a-zA-Z][a-zA-Z0-9_]*(\\.[a-zA-Z][a-zA-Z0-9_]*)+$")
    private val DESKTOP_EXTENSIONS = setOf(".exe", ".desktop", ".bat", ".cmd", ".dll", ".so", ".sh", ".bin", ".msi")

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

    fun isValidPackageName(packageName: String): Boolean {
        val trimmed = packageName.trim()
        if (trimmed.length > 255 || trimmed.isEmpty()) return false
        val lower = trimmed.lowercase()
        if (DESKTOP_EXTENSIONS.any { lower.endsWith(it) }) return false
        if (trimmed.contains("/") || trimmed.contains("\\") || trimmed.contains(":") || trimmed.contains(" ")) return false
        return PACKAGE_NAME_REGEX.matches(trimmed)
    }

    sealed class ValidationResult {
        object Valid : ValidationResult()
        data class Invalid(val reason: String) : ValidationResult()
    }

    fun validatePackageForTargeting(
        packageName: String,
        allowedPackages: Set<String> = BlockerEngine.ALWAYS_ALLOWED_PACKAGES
    ): ValidationResult {
        val trimmed = packageName.trim()
        if (trimmed.isBlank()) {
            return ValidationResult.Invalid("Package name cannot be empty.")
        }
        if (!isValidPackageName(trimmed)) {
            return ValidationResult.Invalid("Invalid package format. Must follow reverse-domain format (e.g. com.example.app).")
        }
        if (isSystemOrExcluded(trimmed, allowedPackages)) {
            return ValidationResult.Invalid("System, safety, emergency, and launcher applications cannot be restricted.")
        }
        return ValidationResult.Valid
    }

    fun isSystemOrExcluded(
        packageName: String,
        allowedPackages: Set<String> = BlockerEngine.ALWAYS_ALLOWED_PACKAGES
    ): Boolean {
        val clean = packageName.trim()
        if (clean.isBlank()) return true
        val lower = clean.lowercase()
        if (lower == "android") return true
        if (allowedPackages.any { it.equals(clean, ignoreCase = true) }) return true

        // Universal launcher suffix and infix protection to prevent trapping users in lock loops
        if (lower.endsWith(".launcher") || lower.endsWith(".launcher3") || lower.contains(".launcher.")) {
            return true
        }

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

    fun resolveCategory(packageName: String, label: String = ""): AppCategory {
        val cleanPkg = packageName.trim().lowercase()
        val cleanLabel = label.trim().lowercase()

        // 1. Social
        if (cleanPkg.contains("instagram") || cleanPkg.contains("twitter") || cleanPkg.contains("tiktok") ||
            cleanPkg.contains("musically") || cleanPkg.contains("snapchat") || cleanPkg.contains("facebook") ||
            cleanPkg.contains("katana") || cleanPkg.contains("reddit") || cleanPkg.contains("pinterest") ||
            cleanPkg.contains("linkedin") || cleanLabel.contains("instagram") || cleanLabel.contains("twitter") ||
            cleanLabel.contains("tiktok") || cleanLabel.contains("snapchat") || cleanLabel.contains("reddit")
        ) {
            return AppCategory.SOCIAL
        }

        // 2. Entertainment / Streaming
        if (cleanPkg.contains("youtube") || cleanPkg.contains("netflix") || cleanPkg.contains("twitch") ||
            cleanPkg.contains("spotify") || cleanPkg.contains("disney") || cleanPkg.contains("hulu") ||
            cleanPkg.contains("primevideo") || cleanPkg.contains("hotstar") || cleanLabel.contains("youtube") ||
            cleanLabel.contains("netflix") || cleanLabel.contains("twitch") || cleanLabel.contains("spotify")
        ) {
            return AppCategory.ENTERTAINMENT
        }

        // 3. Messaging
        if (cleanPkg.contains("whatsapp") || cleanPkg.contains("discord") || cleanPkg.contains("telegram") ||
            cleanPkg.contains("messenger") || cleanPkg.contains("signal") || cleanPkg.contains("orca") ||
            cleanLabel.contains("whatsapp") || cleanLabel.contains("discord") || cleanLabel.contains("telegram")
        ) {
            return AppCategory.MESSAGING
        }

        // 4. Gaming
        if (cleanPkg.contains("roblox") || cleanPkg.contains("pubg") || cleanPkg.contains("game") ||
            cleanPkg.contains("clash") || cleanPkg.contains("minecraft") || cleanPkg.contains("candycrush") ||
            cleanPkg.contains("supercell") || cleanPkg.contains("riotgames") || cleanPkg.contains("epicgames") ||
            cleanLabel.contains("game") || cleanLabel.contains("roblox") || cleanLabel.contains("pubg")
        ) {
            return AppCategory.GAMING
        }

        return AppCategory.OTHER
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

            val appInfo = try {
                info.activityInfo?.applicationInfo
            } catch (_: Exception) {
                null
            }

            var resolvedCat = resolveCategory(pkg, label)
            if (resolvedCat == AppCategory.OTHER && appInfo != null) {
                resolvedCat = when (appInfo.category) {
                    ApplicationInfo.CATEGORY_GAME -> AppCategory.GAMING
                    ApplicationInfo.CATEGORY_SOCIAL -> AppCategory.SOCIAL
                    ApplicationInfo.CATEGORY_VIDEO,
                    ApplicationInfo.CATEGORY_AUDIO -> AppCategory.ENTERTAINMENT
                    else -> AppCategory.OTHER
                }
            }

            apps.add(InstalledAppInfo(packageName = pkg, label = label, icon = icon, category = resolvedCat))
        }

        return apps.sortedBy { it.label.lowercase() }
    }

    fun getCatalogApps(): List<InstalledAppInfo> {
        return TargetCatalog.ENTRIES.map { entry ->
            val primaryPkg = entry.packageNames.firstOrNull() ?: entry.name.lowercase()
            InstalledAppInfo(
                packageName = primaryPkg,
                label = entry.name,
                icon = null,
                category = resolveCategory(primaryPkg, entry.name)
            )
        }
    }

    fun filterApps(
        apps: List<InstalledAppInfo>,
        query: String?,
        category: AppCategory = AppCategory.ALL
    ): List<InstalledAppInfo> {
        val matchesCategory = if (category == AppCategory.ALL) {
            apps
        } else {
            apps.filter { it.category == category }
        }

        if (query.isNullOrBlank()) return matchesCategory
        val cleanQuery = query.trim().lowercase()
        return matchesCategory.filter {
            it.label.lowercase().contains(cleanQuery) ||
            it.packageName.lowercase().contains(cleanQuery) ||
            it.category.title.lowercase().contains(cleanQuery)
        }
    }
}
