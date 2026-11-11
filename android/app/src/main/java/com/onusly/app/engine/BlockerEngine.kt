package com.onusly.app.engine

import com.onusly.app.data.model.BlockEvaluationResult
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.data.model.TargetCatalog

object BlockerEngine {

    val ALWAYS_ALLOWED_PACKAGES = setOf(
        // Core ONUSLY and Android system framework
        "com.onusly.app",
        "android",
        "com.android.systemui",
        "com.android.settings",

        // Google / AOSP telephony, safety & emergency
        "com.android.phone",
        "com.android.dialer",
        "com.android.incallui",
        "com.android.server.telecom",
        "com.android.cellbroadcastreceiver",
        "com.android.emergency",
        "com.android.launcher",
        "com.android.launcher2",
        "com.android.launcher3",
        "com.google.android.dialer",
        "com.google.android.apps.safetyhub",
        "com.google.android.cellbroadcastreceiver",
        "com.google.android.emergency",
        "com.google.android.apps.nexuslauncher",

        // Samsung dialer, in-call UI, emergency & launcher
        "com.samsung.android.dialer",
        "com.samsung.android.incallui",
        "com.samsung.android.app.telephonyui",
        "com.samsung.android.emergency",
        "com.sec.android.app.launcher",
        "com.sec.android.app.dialertab",
        "com.sec.android.emergencyalert",
        "com.sec.android.emergencylauncher",

        // Xiaomi / MIUI dialer, in-call UI, emergency & launcher
        "com.miui.home",
        "com.miui.incallui",
        "com.miui.emergency",

        // Oppo / ColorOS dialer, emergency, safety & launcher
        "com.oppo.launcher",
        "com.oppo.emergency",
        "com.coloros.dialer",
        "com.coloros.safecenter",

        // OnePlus dialer, emergency & launcher
        "com.oneplus.launcher",
        "com.oneplus.dialer",
        "com.oneplus.emergency",

        // Vivo / BBK dialer, in-call UI, emergency & launcher
        "com.vivo.launcher",
        "com.vivo.incallui",
        "com.vivo.dialer",
        "com.vivo.emergency",
        "com.bbk.launcher2",

        // Huawei dialer, in-call UI, emergency & launcher
        "com.huawei.android.launcher",
        "com.huawei.android.incallui",
        "com.huawei.emergency",

        // Motorola dialer, in-call UI, emergency & launcher
        "com.motorola.launcher",
        "com.motorola.launcher3",
        "com.motorola.incallui",
        "com.motorola.emergency",
        "com.motorola.dialer",

        // Additional OEM & Third-Party Launchers (Loop Protection)
        "com.nothing.launcher",
        "com.sonymobile.home",
        "com.asus.launcher",
        "com.transsion.hilauncher",
        "com.transsion.itel.launcher",
        "com.teslacoilsw.launcher",
        "com.microsoft.launcher",
        "bitpit.launcher",
        "ginlemon.flowerfree",
        "app.lawnchair",
        "app.lawnchair.play",
        "ch.deletescape.lawnchair",
        "com.mi.android.globallauncher"
    )

    fun isEnforcingStatus(status: String?): Boolean {
        if (status.isNullOrBlank()) return false
        val normalized = status.trim().lowercase()
        return normalized == "active" || normalized == "proof_submitted" || normalized == "rejected"
    }

    fun evaluatePackage(
        packageName: String?,
        cachedBlocklist: CachedBlocklist?,
        allowedPackages: Set<String> = ALWAYS_ALLOWED_PACKAGES
    ): BlockEvaluationResult {
        if (packageName.isNullOrBlank()) {
            return BlockEvaluationResult(isBlocked = false, matchingTargets = emptyList())
        }

        val normalizedPkg = packageName.trim()

        if (allowedPackages.any { it.equals(normalizedPkg, ignoreCase = true) } ||
            InstalledAppsManager.isSystemOrExcluded(normalizedPkg, allowedPackages)) {
            return BlockEvaluationResult(isBlocked = false, matchingTargets = emptyList())
        }

        if (cachedBlocklist == null || cachedBlocklist.targets.isEmpty()) {
            return BlockEvaluationResult(isBlocked = false, matchingTargets = emptyList())
        }

        val matchingTargets = mutableListOf<BlocklistTarget>()

        for (target in cachedBlocklist.targets) {
            if (!isEnforcingStatus(target.status)) {
                continue
            }

            var matches = false

            // D12 & D6.1: Strict Target Typing
            if (target.applicationTargets.isNotEmpty()) {
                // Consume ONLY targets of kind "android_package".
                // Strictly IGNORE targets of kind "desktop_app" (even reverse-DNS desktop names like org.gnome.Calculator)
                // and any unknown target kinds to prevent cross-platform collisions!
                matches = target.applicationTargets.any { at ->
                    if (!at.kind.equals("android_package", ignoreCase = true)) {
                        false
                    } else {
                        val cleanId = at.identifier.trim()
                        cleanId.equals(normalizedPkg, ignoreCase = true) ||
                        TargetCatalog.getPackagesForAppOrAlias(cleanId).any { it.equals(normalizedPkg, ignoreCase = true) }
                    }
                }
            } else {
                // Backward-compatibility fallback for legacy policies lacking ApplicationTargets:
                // Consume only identifiers without desktop binary extensions (.exe, .desktop, .bat, .app).
                matches = target.apps.any { appItem ->
                    val cleanApp = appItem.trim()
                    val lower = cleanApp.lowercase()
                    if (lower.endsWith(".exe") || lower.endsWith(".desktop") || lower.endsWith(".bat") || lower.endsWith(".app")) {
                        false
                    } else {
                        cleanApp.equals(normalizedPkg, ignoreCase = true) ||
                        TargetCatalog.getPackagesForAppOrAlias(cleanApp).any { it.equals(normalizedPkg, ignoreCase = true) }
                    }
                }
            }

            // Also check domain-associated apps
            if (!matches) {
                matches = target.domains.any { domain ->
                    TargetCatalog.getAppsForDomain(domain).any { it.equals(normalizedPkg, ignoreCase = true) }
                }
            }

            if (matches) {
                matchingTargets.add(target)
            }
        }

        return BlockEvaluationResult(
            isBlocked = matchingTargets.isNotEmpty(),
            matchingTargets = matchingTargets
        )
    }
}
