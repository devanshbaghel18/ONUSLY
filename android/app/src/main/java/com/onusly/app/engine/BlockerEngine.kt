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
        "com.motorola.dialer"
    )

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
            val status = target.status.lowercase().trim()
            if (status != "active" && status != "proof_submitted") {
                continue
            }

            val matchesApp = target.apps.any { it.equals(normalizedPkg, ignoreCase = true) }
            val matchesDomain = target.domains.any { domain ->
                TargetCatalog.getAppsForDomain(domain).any { it.equals(normalizedPkg, ignoreCase = true) }
            }

            if (matchesApp || matchesDomain) {
                matchingTargets.add(target)
            }
        }

        return BlockEvaluationResult(
            isBlocked = matchingTargets.isNotEmpty(),
            matchingTargets = matchingTargets
        )
    }
}
