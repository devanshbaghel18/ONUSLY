package com.onusly.app

import android.content.SharedPreferences
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import com.onusly.app.data.model.BlocklistResponse
import com.onusly.app.data.model.BlocklistTarget
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class BlocklistCacheTest {

    private lateinit var fakePrefs: FakeSharedPreferences
    private lateinit var cache: SharedPreferencesBlocklistCache

    @Before
    fun setUp() {
        fakePrefs = FakeSharedPreferences()
        cache = SharedPreferencesBlocklistCache(fakePrefs)
    }

    @Test
    fun testInitialCacheIsNull() {
        assertNull(cache.getCache())
        assertNull(cache.getRawCachedUserId())
    }

    @Test
    fun testSaveAndRetrieveCacheForSameUser() {
        val response = BlocklistResponse(
            userId = "user_alice",
            version = "v1-1111222233334444",
            generatedAt = "2026-10-08T12:00:00Z",
            targets = listOf(
                BlocklistTarget("g1", "Goal 1", "active", listOf("com.app.a"), listOf("a.com"))
            )
        )

        val saved = cache.saveCache(response, expectedUserId = "user_alice")
        assertTrue(saved)

        val retrieved = cache.getCache("user_alice")
        assertNotNull(retrieved)
        assertEquals("user_alice", retrieved!!.userId)
        assertEquals("v1-1111222233334444", retrieved.version)
        assertEquals(1, retrieved.targets.size)
        assertEquals("com.app.a", retrieved.targets[0].apps[0])
    }

    @Test
    fun testUserIsolation_DifferentUserCannotOverwriteExistingCache() {
        val aliceResponse = BlocklistResponse(
            userId = "user_alice",
            version = "v1-alice",
            generatedAt = "2026-10-08T12:00:00Z",
            targets = listOf(
                BlocklistTarget("g1", "Alice Goal", "active", listOf("com.alice.app"), listOf())
            )
        )
        assertTrue(cache.saveCache(aliceResponse, expectedUserId = "user_alice"))

        // Bob tries to save
        val bobResponse = BlocklistResponse(
            userId = "user_bob",
            version = "v1-bob",
            generatedAt = "2026-10-08T12:00:00Z",
            targets = listOf(
                BlocklistTarget("g2", "Bob Goal", "active", listOf("com.bob.app"), listOf())
            )
        )

        // Attempting to save Bob's response over Alice's non-empty cache without clear must fail
        val bobSaved = cache.saveCache(bobResponse, expectedUserId = "user_bob")
        assertFalse("Bob must not be able to overwrite Alice's cache", bobSaved)

        // Cache still belongs to Alice
        val aliceCache = cache.getCache("user_alice")
        assertNotNull(aliceCache)
        assertEquals("user_alice", aliceCache!!.userId)

        // Querying cache for Bob returns null
        val bobCache = cache.getCache("user_bob")
        assertNull("Querying Bob on Alice's cache must return null", bobCache)
    }

    @Test
    fun testUserIsolation_RejectResponseWhenUserIdMismatch() {
        val response = BlocklistResponse(
            userId = "user_charlie",
            version = "v1-charlie",
            generatedAt = "2026-10-08T12:00:00Z",
            targets = emptyList()
        )

        // Mismatched expectedUserId
        val saved = cache.saveCache(response, expectedUserId = "user_david")
        assertFalse("Must reject when response.userId != expectedUserId", saved)
    }

    @Test
    fun testClearRemovesCacheCompletely() {
        val response = BlocklistResponse(
            userId = "user_alice",
            version = "v1-alice",
            generatedAt = "2026-10-08T12:00:00Z",
            targets = listOf(BlocklistTarget("g1", "Goal", "active", listOf("com.app"), emptyList()))
        )
        cache.saveCache(response, "user_alice")
        assertNotNull(cache.getCache("user_alice"))

        cache.clear()
        assertNull(cache.getCache())
        assertNull(cache.getCache("user_alice"))
        assertNull(cache.getRawCachedUserId())

        // Now Bob can save since cache was cleared on sign-out
        val bobResponse = BlocklistResponse(
            userId = "user_bob",
            version = "v1-bob",
            generatedAt = "2026-10-08T12:00:00Z",
            targets = emptyList()
        )
        assertTrue(cache.saveCache(bobResponse, "user_bob"))
        assertNotNull(cache.getCache("user_bob"))
    }

    @Test
    fun testUpdateLastSyncTimestamp() {
        val response = BlocklistResponse(
            userId = "user_alice",
            version = "v1-etag1",
            generatedAt = "2026-10-08T12:00:00Z",
            targets = emptyList()
        )
        cache.saveCache(response, "user_alice")

        val newTimestamp = 1791465000000L
        cache.updateLastSyncTimestamp(newTimestamp)

        val updated = cache.getCache("user_alice")
        assertNotNull(updated)
        assertEquals(newTimestamp, updated!!.lastSyncTimestamp)
        assertEquals("v1-etag1", updated.version)
    }
}

/**
 * Lightweight in-memory implementation of SharedPreferences for fast JVM unit tests.
 */
class FakeSharedPreferences : SharedPreferences {
    private val data = mutableMapOf<String, Any?>()

    override fun getAll(): Map<String, *> = data
    override fun getString(key: String?, defValue: String?): String? = (data[key] as? String) ?: defValue
    override fun getStringSet(key: String?, defValues: Set<String>?): Set<String>? = (data[key] as? Set<String>) ?: defValues
    override fun getInt(key: String?, defValue: Int): Int = (data[key] as? Int) ?: defValue
    override fun getLong(key: String?, defValue: Long): Long = (data[key] as? Long) ?: defValue
    override fun getFloat(key: String?, defValue: Float): Float = (data[key] as? Float) ?: defValue
    override fun getBoolean(key: String?, defValue: Boolean): Boolean = (data[key] as? Boolean) ?: defValue
    override fun contains(key: String?): Boolean = data.containsKey(key)
    override fun edit(): SharedPreferences.Editor = FakeEditor(data)
    override fun registerOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) {}
    override fun unregisterOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) {}

    class FakeEditor(private val backingMap: MutableMap<String, Any?>) : SharedPreferences.Editor {
        private val pending = mutableMapOf<String, Any?>()
        private val removes = mutableSetOf<String>()
        private var clearAll = false

        override fun putString(key: String?, value: String?): SharedPreferences.Editor {
            key?.let { pending[it] = value }
            return this
        }
        override fun putStringSet(key: String?, values: Set<String>?): SharedPreferences.Editor {
            key?.let { pending[it] = values }
            return this
        }
        override fun putInt(key: String?, value: Int): SharedPreferences.Editor {
            key?.let { pending[it] = value }
            return this
        }
        override fun putLong(key: String?, value: Long): SharedPreferences.Editor {
            key?.let { pending[it] = value }
            return this
        }
        override fun putFloat(key: String?, value: Float): SharedPreferences.Editor {
            key?.let { pending[it] = value }
            return this
        }
        override fun putBoolean(key: String?, value: Boolean): SharedPreferences.Editor {
            key?.let { pending[it] = value }
            return this
        }
        override fun remove(key: String?): SharedPreferences.Editor {
            key?.let { removes.add(it) }
            return this
        }
        override fun clear(): SharedPreferences.Editor {
            clearAll = true
            return this
        }
        override fun commit(): Boolean {
            apply()
            return true
        }
        override fun apply() {
            if (clearAll) backingMap.clear()
            removes.forEach { backingMap.remove(it) }
            backingMap.putAll(pending)
        }
    }
}
