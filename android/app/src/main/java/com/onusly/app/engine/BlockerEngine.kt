package com.onusly.app.engine

import com.onusly.app.data.model.BlockEvaluationResult
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist

object BlockerEngine {

    val ALWAYS_ALLOWED_PACKAGES = setOf(
        "com.onusly.app",
        "android",
        "com.android.systemui",
        "com.android.settings",
        "com.google.android.apps.nexuslauncher",
        "com.android.launcher3"
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

        if (allowedPackages.contains(normalizedPkg)) {
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

            if (target.apps.any { it.equals(normalizedPkg, ignoreCase = false) }) {
                matchingTargets.add(target)
            }
        }

        return BlockEvaluationResult(
            isBlocked = matchingTargets.isNotEmpty(),
            matchingTargets = matchingTargets
        )
    }
}
