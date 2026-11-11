# ProGuard / R8 Rules for ONUSLY Android Release Candidate

# Preserve Data Models and JSON Serialization/Deserialization
-keepclassmembers class * {
    @org.json.* <fields>;
    @org.json.* <methods>;
}

-keep class com.onusly.app.data.model.** { *; }
-keepclassmembers class com.onusly.app.data.model.** { *; }

# Preserve API Interfaces and Networking
-keep class com.onusly.app.data.api.** { *; }
-keepclassmembers class com.onusly.app.data.api.** { *; }

# Preserve Blocker Engine and Installed Apps Manager
-keep class com.onusly.app.engine.** { *; }
-keepclassmembers class com.onusly.app.engine.** { *; }

# Preserve Accessibility Service and Broadcast Receiver
-keep class com.onusly.app.service.OnuslyAccessibilityService { *; }
-keep class com.onusly.app.receiver.BootReceiver { *; }

# OkHttp Rules
-dontwarn okhttp3.**
-dontwarn okio.**
-keep class okhttp3.** { *; }
-keep interface okhttp3.** { *; }
-keep class okio.** { *; }

# Kotlin Coroutines
-dontwarn kotlinx.coroutines.**
-keep class kotlinx.coroutines.** { *; }

# Android Security & Crypto
-keep class androidx.security.crypto.** { *; }

# Google Play Services Auth
-dontwarn com.google.android.gms.**
-keep class com.google.android.gms.auth.api.** { *; }
-keep class com.google.android.gms.common.** { *; }
