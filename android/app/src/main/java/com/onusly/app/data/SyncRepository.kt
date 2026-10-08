package com.onusly.app.data

import android.content.Context
import android.content.SharedPreferences
import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.cache.BlocklistCache
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.data.model.SyncResult
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

class SyncRepository(
    private val authManager: AuthManager,
    private val blocklistCache: BlocklistCache,
    private val apiClient: EnforcementApiClient,
    private val settingsPrefs: SharedPreferences
) {

    companion object {
        const val DEFAULT_BASE_URL = "http://10.0.2.2:8080"
        const val KEY_BASE_URL = "api_base_url"
        private const val SETTINGS_PREFS_NAME = "onusly_settings"

        fun create(context: Context): SyncRepository {
            val authManager = AuthManager.create(context)
            val cache = SharedPreferencesBlocklistCache.create(context)
            val apiClient = EnforcementApiClient()
            val settingsPrefs = context.getSharedPreferences(SETTINGS_PREFS_NAME, Context.MODE_PRIVATE)
            return SyncRepository(authManager, cache, apiClient, settingsPrefs)
        }
    }

    private val _cachedBlocklistFlow = MutableStateFlow<CachedBlocklist?>(null)
    val cachedBlocklistFlow: StateFlow<CachedBlocklist?> = _cachedBlocklistFlow.asStateFlow()

    init {
        refreshFlowFromCache()
    }

    fun getBaseUrl(): String {
        return settingsPrefs.getString(KEY_BASE_URL, DEFAULT_BASE_URL) ?: DEFAULT_BASE_URL
    }

    fun setBaseUrl(url: String) {
        settingsPrefs.edit().putString(KEY_BASE_URL, url.trim().trimEnd('/')).apply()
    }

    fun getActiveCache(): CachedBlocklist? {
        val currentUserId = authManager.getUserId()
        return blocklistCache.getCache(currentUserId)
    }

    fun refreshFlowFromCache() {
        _cachedBlocklistFlow.value = getActiveCache()
    }

    suspend fun syncBlocklist(): SyncResult {
        val session = authManager.getSession()
        if (session == null || session.jwtToken.isBlank() || session.userId.isBlank()) {
            _cachedBlocklistFlow.value = null
            return SyncResult.Error(401, "User is not signed in")
        }

        val currentUserId = session.userId
        val existingCache = blocklistCache.getCache(currentUserId)
        val ifNoneMatch = existingCache?.version

        val result = apiClient.fetchBlocklist(
            baseUrl = getBaseUrl(),
            jwtToken = session.jwtToken,
            ifNoneMatch = ifNoneMatch
        )

        when (result) {
            is SyncResult.Success -> {
                if (result.response.userId != currentUserId) {
                    return SyncResult.Error(403, "Response userId does not match authenticated user")
                }
                val saved = blocklistCache.saveCache(result.response, currentUserId)
                if (saved) {
                    refreshFlowFromCache()
                }
            }
            is SyncResult.NotModified -> {
                blocklistCache.updateLastSyncTimestamp(System.currentTimeMillis())
                refreshFlowFromCache()
            }
            is SyncResult.Error -> {
                // Keep existing cache
                refreshFlowFromCache()
            }
        }

        return result
    }

    fun onUserSignOut() {
        blocklistCache.clear()
        authManager.clearSession()
        _cachedBlocklistFlow.value = null
    }
}
