package com.onusly.app.data.cache

import android.content.Context
import android.content.SharedPreferences
import com.onusly.app.data.model.BlocklistParser
import com.onusly.app.data.model.BlocklistResponse
import com.onusly.app.data.model.CachedBlocklist

interface BlocklistCache {
    fun getCache(expectedUserId: String? = null): CachedBlocklist?
    fun saveCache(response: BlocklistResponse, expectedUserId: String): Boolean
    fun updateLastSyncTimestamp(timestamp: Long)
    fun clear()
    fun getRawCachedUserId(): String?
}

class SharedPreferencesBlocklistCache(
    private val prefs: SharedPreferences
) : BlocklistCache {

    companion object {
        private const val KEY_CACHED_JSON = "cached_blocklist_json"
        private const val KEY_CACHED_USER_ID = "cached_user_id"
        private const val PREFS_NAME = "onusly_blocklist_cache"

        fun create(context: Context): SharedPreferencesBlocklistCache {
            val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            return SharedPreferencesBlocklistCache(prefs)
        }
    }

    @Synchronized
    override fun getCache(expectedUserId: String?): CachedBlocklist? {
        val json = prefs.getString(KEY_CACHED_JSON, null) ?: return null
        val cached = BlocklistParser.parseCached(json) ?: return null

        if (expectedUserId != null && expectedUserId.isNotBlank()) {
            if (cached.userId != expectedUserId) {
                return null
            }
        }
        return cached
    }

    @Synchronized
    override fun saveCache(response: BlocklistResponse, expectedUserId: String): Boolean {
        if (response.userId.isBlank() || expectedUserId.isBlank()) {
            return false
        }
        if (response.userId != expectedUserId) {
            return false
        }

        val existingUserId = prefs.getString(KEY_CACHED_USER_ID, null)
        if (!existingUserId.isNullOrBlank() && existingUserId != expectedUserId) {
            // Cannot overwrite existing non-empty cache belonging to another user
            return false
        }

        val cached = CachedBlocklist(
            userId = response.userId,
            version = response.version,
            generatedAt = response.generatedAt,
            lastSyncTimestamp = System.currentTimeMillis(),
            targets = response.targets
        )

        val json = BlocklistParser.cachedToJson(cached)
        prefs.edit()
            .putString(KEY_CACHED_JSON, json)
            .putString(KEY_CACHED_USER_ID, response.userId)
            .apply()

        return true
    }

    @Synchronized
    override fun updateLastSyncTimestamp(timestamp: Long) {
        val current = getCache() ?: return
        val updated = current.copy(lastSyncTimestamp = timestamp)
        val json = BlocklistParser.cachedToJson(updated)
        prefs.edit()
            .putString(KEY_CACHED_JSON, json)
            .apply()
    }

    @Synchronized
    override fun clear() {
        prefs.edit()
            .remove(KEY_CACHED_JSON)
            .remove(KEY_CACHED_USER_ID)
            .apply()
    }

    @Synchronized
    override fun getRawCachedUserId(): String? {
        return prefs.getString(KEY_CACHED_USER_ID, null)
    }
}
