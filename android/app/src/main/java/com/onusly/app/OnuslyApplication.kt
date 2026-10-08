package com.onusly.app

import android.app.Application
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import com.onusly.app.data.SyncRepository
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

class OnuslyApplication : Application() {

    val applicationScope = CoroutineScope(SupervisorJob() + Dispatchers.Main)

    lateinit var authManager: AuthManager
        private set
    lateinit var blocklistCache: SharedPreferencesBlocklistCache
        private set
    lateinit var syncRepository: SyncRepository
        private set

    override fun onCreate() {
        super.onCreate()
        instance = this

        authManager = AuthManager.create(this)
        blocklistCache = SharedPreferencesBlocklistCache.create(this)
        syncRepository = SyncRepository.create(this)

        // Startup sync if signed in
        if (authManager.isSignedIn()) {
            applicationScope.launch(Dispatchers.IO) {
                syncRepository.syncBlocklist()
            }
        }

        registerNetworkCallback()
    }

    private fun registerNetworkCallback() {
        try {
            val cm = getSystemService(ConnectivityManager::class.java) ?: return
            val request = NetworkRequest.Builder()
                .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                .build()

            cm.registerNetworkCallback(request, object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) {
                    if (authManager.isSignedIn()) {
                        applicationScope.launch(Dispatchers.IO) {
                            syncRepository.syncBlocklist()
                        }
                    }
                }
            })
        } catch (_: Exception) {
        }
    }

    companion object {
        lateinit var instance: OnuslyApplication
            private set
    }
}
