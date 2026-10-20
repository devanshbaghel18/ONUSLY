import java.util.Properties

plugins {
    id("com.android.application")
}

fun getGoogleClientId(): String {
    val envVal = System.getenv("GOOGLE_CLIENT_ID")
    if (!envVal.isNullOrBlank()) return envVal

    val localPropsFile = rootProject.file("local.properties")
    if (localPropsFile.exists()) {
        val props = Properties()
        localPropsFile.inputStream().use { stream ->
            props.load(stream)
        }
        val propVal = props.getProperty("google.client.id") ?: props.getProperty("GOOGLE_CLIENT_ID")
        if (!propVal.isNullOrBlank()) return propVal
    }

    val dotEnvFile = rootProject.file("../.env")
    if (dotEnvFile.exists()) {
        dotEnvFile.readLines().forEach { line ->
            val trimmed = line.trim()
            if (trimmed.startsWith("GOOGLE_CLIENT_ID=")) {
                val value = trimmed.substringAfter("GOOGLE_CLIENT_ID=").trim().trim('"').trim('\'')
                if (value.isNotBlank()) return value
            }
        }
    }
    return ""
}

android {
    namespace = "com.onusly.app"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.onusly.app"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "1.0.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        val googleClientId = getGoogleClientId()
        buildConfigField("String", "GOOGLE_SERVER_CLIENT_ID", "\"$googleClientId\"")
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
        debug {
            applicationIdSuffix = ""
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        buildConfig = true
    }

    testOptions {
        unitTests.isReturnDefaultValues = true
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("com.google.android.material:material:1.12.0")
    implementation("androidx.constraintlayout:constraintlayout:2.1.4")
    implementation("androidx.activity:activity-ktx:1.9.3")
    implementation("androidx.fragment:fragment-ktx:1.7.0")
    implementation("androidx.recyclerview:recyclerview:1.3.2")

    // Google Sign-In
    implementation("com.google.android.gms:play-services-auth:21.2.0")

    // Networking & Async
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.8.1")

    // Testing
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.8.1")
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
}
