package com.onusly.app.data.auth

import android.content.Context
import android.content.SharedPreferences
import android.util.Base64
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.google.android.gms.auth.api.signin.GoogleSignIn
import com.google.android.gms.auth.api.signin.GoogleSignInClient
import com.google.android.gms.auth.api.signin.GoogleSignInOptions
import com.onusly.app.data.model.UserSession
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.File

class AuthManager(
    private val context: Context,
    private val prefs: SharedPreferences,
    private val httpClient: OkHttpClient = OkHttpClient()
) {

    companion object {
        val SERVER_CLIENT_ID: String = com.onusly.app.BuildConfig.GOOGLE_SERVER_CLIENT_ID
        const val PREFS_NAME = "onusly_auth_prefs"
        const val ENCRYPTED_PREFS_NAME = "onusly_encrypted_auth_prefs"
        const val KEY_JWT_TOKEN = "jwt_token"
        const val KEY_USER_ID = "user_id"
        const val KEY_EMAIL = "email"
        const val KEY_NAME = "name"

        fun create(context: Context): AuthManager {
            val appContext = context.applicationContext ?: context
            val prefs = getSecurePreferences(appContext)
            migrateFromUnencryptedIfNeeded(appContext, prefs)
            return AuthManager(appContext, prefs)
        }

        private fun createEncryptedPreferences(context: Context): SharedPreferences {
            val masterKey = MasterKey.Builder(context)
                .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
                .build()

            return EncryptedSharedPreferences.create(
                context,
                ENCRYPTED_PREFS_NAME,
                masterKey,
                EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
                EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
            )
        }

        private fun getSecurePreferences(context: Context): SharedPreferences {
            return try {
                createEncryptedPreferences(context)
            } catch (e: Throwable) {
                // If creation fails (e.g. KeyStore corruption or device incompatibility),
                // attempt to wipe corrupted preferences file and recreate once.
                try {
                    deletePreferencesFile(context, ENCRYPTED_PREFS_NAME)
                    createEncryptedPreferences(context)
                } catch (_: Throwable) {
                    // Fallback gracefully (e.g. in unit test environments without AndroidKeyStore
                    // where GeneralSecurityException or NoClassDefFoundError or Exception is thrown)
                    try {
                        context.getSharedPreferences(ENCRYPTED_PREFS_NAME, Context.MODE_PRIVATE)
                    } catch (_: Throwable) {
                        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
                    }
                }
            }
        }

        private fun deletePreferencesFile(context: Context, prefsName: String) {
            try {
                if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.N) {
                    context.deleteSharedPreferences(prefsName)
                }
            } catch (_: Throwable) {
            }
            try {
                val dataDir = context.filesDir?.parentFile
                if (dataDir != null) {
                    val prefsFile = File(dataDir, "shared_prefs/$prefsName.xml")
                    if (prefsFile.exists()) {
                        prefsFile.delete()
                    }
                }
            } catch (_: Throwable) {
            }
        }

        private fun migrateFromUnencryptedIfNeeded(context: Context, securePrefs: SharedPreferences) {
            try {
                val oldPrefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE) ?: return
                if (oldPrefs === securePrefs) {
                    return
                }

                val hasValues = oldPrefs.contains(KEY_JWT_TOKEN) ||
                        oldPrefs.contains(KEY_USER_ID) ||
                        oldPrefs.contains(KEY_EMAIL) ||
                        oldPrefs.contains(KEY_NAME)

                if (hasValues) {
                    val editor = securePrefs.edit()
                    if (oldPrefs.contains(KEY_JWT_TOKEN)) {
                        val token = oldPrefs.getString(KEY_JWT_TOKEN, null)
                        if (token != null) editor.putString(KEY_JWT_TOKEN, token)
                    }
                    if (oldPrefs.contains(KEY_USER_ID)) {
                        val userId = oldPrefs.getString(KEY_USER_ID, null)
                        if (userId != null) editor.putString(KEY_USER_ID, userId)
                    }
                    if (oldPrefs.contains(KEY_EMAIL)) {
                        val email = oldPrefs.getString(KEY_EMAIL, null)
                        if (email != null) editor.putString(KEY_EMAIL, email)
                    }
                    if (oldPrefs.contains(KEY_NAME)) {
                        val name = oldPrefs.getString(KEY_NAME, null)
                        if (name != null) editor.putString(KEY_NAME, name)
                    }
                    editor.apply()

                    oldPrefs.edit().clear().apply()
                    deletePreferencesFile(context, PREFS_NAME)
                }
            } catch (_: Throwable) {
            }
        }

        private fun decodeBase64(input: String): ByteArray? {
            return try {
                java.util.Base64.getUrlDecoder().decode(input)
            } catch (_: Throwable) {
                try {
                    java.util.Base64.getDecoder().decode(input)
                } catch (_: Throwable) {
                    try {
                        Base64.decode(input, Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP)
                    } catch (_: Throwable) {
                        null
                    }
                }
            }
        }

        fun parseUserIdFromJwt(jwt: String): String? {
            return try {
                val parts = jwt.split(".")
                if (parts.size >= 2) {
                    val decoded = decodeBase64(parts[1]) ?: return null
                    val payload = String(decoded, Charsets.UTF_8)
                    val json = JSONObject(payload)
                    val sub = json.optString("sub", "")
                    if (sub.isNotBlank()) sub else null
                } else null
            } catch (e: Exception) {
                null
            }
        }

        fun isTokenExpired(token: String?): Boolean {
            if (token.isNullOrBlank()) return true
            return try {
                val parts = token.split(".")
                if (parts.size >= 2) {
                    val decoded = decodeBase64(parts[1]) ?: return false
                    val payload = String(decoded, Charsets.UTF_8)
                    val json = JSONObject(payload)
                    if (json.has("exp")) {
                        var expSeconds = json.optLong("exp", 0L)
                        if (expSeconds > 100_000_000_000L) {
                            expSeconds /= 1000
                        }
                        val currentSeconds = System.currentTimeMillis() / 1000
                        currentSeconds >= expSeconds
                    } else {
                        false
                    }
                } else {
                    false
                }
            } catch (_: Throwable) {
                false
            }
        }
    }

    private val tokenExpirationListeners = mutableListOf<() -> Unit>()

    fun addTokenExpirationListener(listener: () -> Unit) {
        synchronized(tokenExpirationListeners) {
            tokenExpirationListeners.add(listener)
        }
    }

    fun removeTokenExpirationListener(listener: () -> Unit) {
        synchronized(tokenExpirationListeners) {
            tokenExpirationListeners.remove(listener)
        }
    }

    fun notifyTokenExpired() {
        clearSession()
        val listeners = synchronized(tokenExpirationListeners) {
            tokenExpirationListeners.toList()
        }
        for (listener in listeners) {
            try {
                listener()
            } catch (_: Throwable) {
            }
        }
    }

    fun isTokenExpired(token: String?): Boolean = Companion.isTokenExpired(token)

    fun isTokenExpired(): Boolean {
        val token = prefs.getString(KEY_JWT_TOKEN, null)
        if (token != null && Companion.isTokenExpired(token)) {
            notifyTokenExpired()
            return true
        }
        return Companion.isTokenExpired(token)
    }

    fun getGoogleSignInClient(): GoogleSignInClient {
        val gso = GoogleSignInOptions.Builder(GoogleSignInOptions.DEFAULT_SIGN_IN)
            .requestIdToken(SERVER_CLIENT_ID)
            .requestEmail()
            .requestProfile()
            .build()
        return GoogleSignIn.getClient(context, gso)
    }

    fun getSession(): UserSession? {
        val token = getJwtToken() ?: return null
        val userId = getUserId() ?: return null
        val email = prefs.getString(KEY_EMAIL, "") ?: ""
        val name = prefs.getString(KEY_NAME, "") ?: ""
        return UserSession(
            userId = userId,
            email = email,
            name = name,
            jwtToken = token
        )
    }

    fun getUserId(): String? {
        return prefs.getString(KEY_USER_ID, null)
    }

    fun getJwtToken(): String? {
        val token = prefs.getString(KEY_JWT_TOKEN, null)
        if (token != null && isTokenExpired(token)) {
            notifyTokenExpired()
            return null
        }
        return token
    }

    fun isSignedIn(): Boolean {
        val token = prefs.getString(KEY_JWT_TOKEN, null)
        val userId = getUserId()
        if (token.isNullOrBlank() || userId.isNullOrBlank()) {
            return false
        }
        if (isTokenExpired(token)) {
            notifyTokenExpired()
            return false
        }
        return true
    }

    fun saveSession(session: UserSession) {
        prefs.edit()
            .putString(KEY_JWT_TOKEN, session.jwtToken)
            .putString(KEY_USER_ID, session.userId)
            .putString(KEY_EMAIL, session.email)
            .putString(KEY_NAME, session.name)
            .apply()
    }

    fun clearSession() {
        prefs.edit()
            .remove(KEY_JWT_TOKEN)
            .remove(KEY_USER_ID)
            .remove(KEY_EMAIL)
            .remove(KEY_NAME)
            .apply()

        try {
            getGoogleSignInClient().signOut()
        } catch (_: Throwable) {
        }
    }

    suspend fun exchangeGoogleIdToken(apiBaseUrl: String, googleIdToken: String): Result<UserSession> = withContext(Dispatchers.IO) {
        try {
            val jsonBody = JSONObject().apply {
                put("idToken", googleIdToken)
            }

            val request = Request.Builder()
                .url("${apiBaseUrl.trimEnd('/')}/auth/google")
                .post(jsonBody.toString().toRequestBody("application/json".toMediaType()))
                .build()

            val response = httpClient.newCall(request).execute()
            val responseBody = response.body?.string() ?: ""

            if (!response.isSuccessful) {
                return@withContext Result.failure(Exception("Auth failed: HTTP ${response.code}: $responseBody"))
            }

            val root = JSONObject(responseBody)
            val token = root.optString("token", "")
            if (token.isBlank()) {
                return@withContext Result.failure(Exception("No JWT token returned from auth server"))
            }

            val userObj = root.optJSONObject("user")
            var userId = userObj?.optString("id", "") ?: ""
            val email = userObj?.optString("email", "") ?: ""
            val name = userObj?.optString("name", "") ?: ""

            if (userId.isBlank()) {
                userId = parseUserIdFromJwt(token) ?: ""
            }

            if (userId.isBlank()) {
                return@withContext Result.failure(Exception("Could not extract user ID from auth response or JWT"))
            }

            val session = UserSession(
                userId = userId,
                email = email,
                name = name,
                jwtToken = token
            )

            saveSession(session)
            Result.success(session)
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
}
