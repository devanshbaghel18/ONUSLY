package com.onusly.app.data.auth

import android.content.Context
import android.content.SharedPreferences
import android.util.Base64
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

class AuthManager(
    private val context: Context,
    private val prefs: SharedPreferences,
    private val httpClient: OkHttpClient = OkHttpClient()
) {

    companion object {
        const val SERVER_CLIENT_ID = "24284302254-n26lia4s6vi33koas28u1i26ejimdfp3.apps.googleusercontent.com"
        private const val PREFS_NAME = "onusly_auth_prefs"
        private const val KEY_JWT_TOKEN = "jwt_token"
        private const val KEY_USER_ID = "user_id"
        private const val KEY_EMAIL = "email"
        private const val KEY_NAME = "name"

        fun create(context: Context): AuthManager {
            val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            return AuthManager(context.applicationContext, prefs)
        }

        fun parseUserIdFromJwt(jwt: String): String? {
            return try {
                val parts = jwt.split(".")
                if (parts.size >= 2) {
                    val payload = String(Base64.decode(parts[1], Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP))
                    val json = JSONObject(payload)
                    val sub = json.optString("sub", "")
                    if (sub.isNotBlank()) sub else null
                } else null
            } catch (e: Exception) {
                null
            }
        }
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
        val token = prefs.getString(KEY_JWT_TOKEN, null) ?: return null
        val userId = prefs.getString(KEY_USER_ID, null) ?: return null
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
        return prefs.getString(KEY_JWT_TOKEN, null)
    }

    fun isSignedIn(): Boolean {
        return !getJwtToken().isNullOrBlank() && !getUserId().isNullOrBlank()
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
        } catch (_: Exception) {
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
