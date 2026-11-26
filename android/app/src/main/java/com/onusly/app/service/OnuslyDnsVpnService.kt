package com.onusly.app.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat
import com.onusly.app.OnuslyApplication
import com.onusly.app.R
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.dns.DnsResolverEngine
import com.onusly.app.dns.DnsResponseCache
import com.onusly.app.dns.DnsShieldManager
import com.onusly.app.dns.DnsWireCodec
import com.onusly.app.dns.IpUdpPacketCodec
import com.onusly.app.dns.UpstreamDnsClient
import com.onusly.app.engine.DomainMatcher
import com.onusly.app.ui.MainActivity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.io.FileInputStream
import java.io.FileOutputStream
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors
import java.util.concurrent.RejectedExecutionException
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Local non-root DNS-only VpnService for system-wide website blocking on Android.
 *
 * Routes ONLY virtual DNS addresses (10.111.222.2/32 and fd00:111:222::2/128) through the TUN
 * interface so normal TCP/HTTPS app traffic flows natively without extra latency or battery drain.
 */
class OnuslyDnsVpnService : VpnService() {

    companion object {
        private const val TAG = "OnuslyDnsVpn"
        const val ACTION_START = "com.onusly.app.dns.START"
        const val ACTION_STOP = "com.onusly.app.dns.STOP"

        private const val CHANNEL_ID = "onusly_dns_shield"
        private const val BLOCK_ALERT_CHANNEL_ID = "onusly_dns_blocks"
        private const val NOTIFICATION_ID = 2001
        private const val BLOCK_ALERT_NOTIFICATION_ID = 2002
        private const val BLOCK_ALERT_COOLDOWN_MS = 20_000L
        private const val HIT_STORE_SAVE_INTERVAL_MS = 15_000L

        private const val VPN_IPV4_ADDRESS = "10.111.222.1"
        private const val VPN_IPV4_DNS = "10.111.222.2"
        private const val VPN_IPV6_ADDRESS = "fd00:111:222::1"
        private const val VPN_IPV6_DNS = "fd00:111:222::2"
        private const val DNS_PORT = 53
        private const val MTU = 1500
    }

    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val workerExecutor = Executors.newFixedThreadPool(8)
    private val runningFlag = AtomicBoolean(false)
    private val tunWriteLock = Any()

    @Volatile
    private var tunInterface: ParcelFileDescriptor? = null

    @Volatile
    private var packetReaderThread: Thread? = null

    private var statsJob: Job? = null
    private var blocklistFlowJob: Job? = null
    private var blocklistPrefs: SharedPreferences? = null
    private lateinit var dnsHitStore: com.onusly.app.dns.DnsHitStore
    private val lastBlockNoticeMs = ConcurrentHashMap<String, Long>()
    @Volatile private var lastHitsSignature: Long = -1L
    @Volatile private var lastHitsSavedMs: Long = 0L

    private val resolverEngine: DnsResolverEngine by lazy {
        val upstream = UpstreamDnsClient(
            protectDatagram = { sock -> protect(sock) },
            protectSocket = { sock -> protect(sock) }
        )
        DnsResolverEngine(
            upstream = upstream,
            cache = DnsResponseCache(),
            onDomainBlocked = { blockedDomain -> notifyDomainBlocked(blockedDomain) }
        )
    }

    private val prefChangeListener = SharedPreferences.OnSharedPreferenceChangeListener { _, _ ->
        reloadPolicyFromApp()
    }

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        dnsHitStore = com.onusly.app.dns.DnsHitStore.create(this)
        val todayKey = com.onusly.app.dns.DnsHitStore.dateKeyFor(System.currentTimeMillis())
        val persisted = dnsHitStore.loadDay(todayKey)
        resolverEngine.seedDayHits(todayKey, persisted)
        DnsShieldManager.updateDomainHits(persisted)

        blocklistPrefs = getSharedPreferences("onusly_blocklist_cache", Context.MODE_PRIVATE)
        blocklistPrefs?.registerOnSharedPreferenceChangeListener(prefChangeListener)

        reloadPolicyFromApp()
        observeBlocklistFlow()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopVpnInternal()
            stopSelf()
            return START_NOT_STICKY
        }

        startVpnInternal()
        return START_STICKY
    }

    private fun startVpnInternal() {
        if (runningFlag.get()) {
            reloadPolicyFromApp()
            updateNotification()
            return
        }

        // Promote to foreground immediately to comply with Android 8+ / Android 14+ FGS requirements
        startForegroundWithStats(0, 0L)

        try {
            val builder = Builder()
                .setSession("ONUSLY Mobile DNS Shield")
                .setMtu(MTU)
                .addAddress(VPN_IPV4_ADDRESS, 32)
                .addDnsServer(VPN_IPV4_DNS)
                .addRoute(VPN_IPV4_DNS, 32)
                .addAddress(VPN_IPV6_ADDRESS, 128)
                .addDnsServer(VPN_IPV6_DNS)
                .addRoute(VPN_IPV6_DNS, 128)

            try {
                builder.addDisallowedApplication(packageName)
            } catch (_: Exception) {
            }

            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                builder.setMetered(false)
            }

            val pfd = builder.establish()
            if (pfd == null) {
                DnsShieldManager.updateError("VPN permission required or revoked by system")
                DnsShieldManager.updateRunning(false)
                stopForegroundCompat()
                stopSelf()
                return
            }

            tunInterface = pfd
            runningFlag.set(true)
            DnsShieldManager.updateError(null)
            DnsShieldManager.updateRunning(true)

            reloadPolicyFromApp()
            startPacketLoop(pfd)
            startStatsLoop()
            Log.i(TAG, "Mobile DNS Shield VPN started (blocked domains=${resolverEngine.getStats().blockedDomainsCount})")
        } catch (e: Exception) {
            Log.e(TAG, "Failed to establish Mobile DNS Shield VPN", e)
            DnsShieldManager.updateError(e.message ?: "VPN establish failed")
            stopVpnInternal()
            stopSelf()
        }
    }

    private fun startPacketLoop(pfd: ParcelFileDescriptor) {
        val thread = Thread({
            val input = FileInputStream(pfd.fileDescriptor)
            val output = FileOutputStream(pfd.fileDescriptor)
            val packetBuf = ByteArray(32767)

            while (runningFlag.get() && !Thread.currentThread().isInterrupted) {
                val readLen = try {
                    input.read(packetBuf)
                } catch (e: Exception) {
                    if (runningFlag.get()) {
                        Log.w(TAG, "TUN read ended: ${e.message}")
                    }
                    break
                }

                if (readLen < 0) break
                if (readLen == 0) continue

                val rawPacket = packetBuf.copyOfRange(0, readLen)
                val udpPacket = IpUdpPacketCodec.parseUdpPacket(rawPacket, readLen) ?: continue
                if (udpPacket.dstPort != DNS_PORT) continue

                if (!runningFlag.get() || workerExecutor.isShutdown) break
                try {
                    workerExecutor.execute {
                        if (!runningFlag.get()) return@execute
                        val dnsResp = try {
                            resolverEngine.handleDnsQuery(udpPacket.payload)
                        } catch (_: Exception) {
                            DnsWireCodec.buildServFailResponse(udpPacket.payload)
                        }

                        val respIpPacket = try {
                            IpUdpPacketCodec.buildUdpResponse(udpPacket, dnsResp)
                        } catch (_: Exception) {
                            return@execute
                        }

                        synchronized(tunWriteLock) {
                            if (!runningFlag.get()) return@synchronized
                            try {
                                output.write(respIpPacket)
                            } catch (_: Exception) {
                            }
                        }
                    }
                } catch (_: RejectedExecutionException) {
                    break
                }
            }
        }, "OnuslyDnsTunReader")
        thread.isDaemon = true
        packetReaderThread = thread
        thread.start()
    }

    private fun startStatsLoop() {
        statsJob?.cancel()
        statsJob = serviceScope.launch {
            while (isActive && runningFlag.get()) {
                val stats = resolverEngine.getStats()
                val hits = resolverEngine.snapshotDailyDomainHits()
                DnsShieldManager.updateStats(stats)
                DnsShieldManager.updateDomainHits(hits)
                if (::dnsHitStore.isInitialized) {
                    val nowMs = System.currentTimeMillis()
                    val sig = hits.fold(hits.size.toLong()) { acc, h ->
                        acc * 31L + h.count + h.opens * 7L + h.blockedAttempts * 13L
                    }
                    if (sig != lastHitsSignature && (nowMs - lastHitsSavedMs >= HIT_STORE_SAVE_INTERVAL_MS)) {
                        dnsHitStore.saveDay(resolverEngine.getCurrentDayKey(), hits)
                        lastHitsSignature = sig
                        lastHitsSavedMs = nowMs
                    }
                }
                updateNotification()
                delay(2_000)
            }
        }
    }

    private fun observeBlocklistFlow() {
        blocklistFlowJob?.cancel()
        val app = application as? OnuslyApplication ?: return
        blocklistFlowJob = serviceScope.launch {
            app.syncRepository.cachedBlocklistFlow.collect { updated ->
                applyPolicy(updated)
            }
        }
    }

    private fun reloadPolicyFromApp() {
        val app = application as? OnuslyApplication ?: return
        applyPolicy(app.syncRepository.getActiveCache())
    }

    private fun applyPolicy(cache: CachedBlocklist?) {
        val app = application as? OnuslyApplication
        val apiHost = DomainMatcher.extractHostFromUrl(app?.syncRepository?.getBaseUrl())
        val extraProtected = if (apiHost.isNotBlank()) setOf(apiHost) else emptySet()
        resolverEngine.updatePolicy(cache, extraProtected)
        val updatedStats = resolverEngine.getStats()
        DnsShieldManager.updateStats(updatedStats)
        if (runningFlag.get()) {
            updateNotification()
        }
    }

    private fun stopVpnInternal() {
        if (::dnsHitStore.isInitialized) {
            try {
                dnsHitStore.saveDay(resolverEngine.getCurrentDayKey(), resolverEngine.snapshotDailyDomainHits())
            } catch (_: Exception) {
            }
        }
        runningFlag.set(false)
        DnsShieldManager.updateRunning(false)

        statsJob?.cancel()
        statsJob = null

        try {
            tunInterface?.close()
        } catch (_: Exception) {
        }
        tunInterface = null

        packetReaderThread?.interrupt()
        packetReaderThread = null

        stopForegroundCompat()
    }

    override fun onRevoke() {
        DnsShieldManager.setUserEnabled(this, false)
        stopVpnInternal()
        super.onRevoke()
        stopSelf()
    }

    override fun onDestroy() {
        stopVpnInternal()
        blocklistPrefs?.unregisterOnSharedPreferenceChangeListener(prefChangeListener)
        blocklistPrefs = null
        blocklistFlowJob?.cancel()
        serviceScope.cancel()
        workerExecutor.shutdownNow()
        super.onDestroy()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val nm = getSystemService(NotificationManager::class.java) ?: return
            val channel = NotificationChannel(
                CHANNEL_ID,
                "ONUSLY Mobile DNS Shield",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Active website blocking notification for ONUSLY Mobile DNS Shield"
                setShowBadge(false)
            }
            nm.createNotificationChannel(channel)

            val alertChannel = NotificationChannel(
                BLOCK_ALERT_CHANNEL_ID,
                "Blocked Website Alerts",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Subtle notification when ONUSLY Mobile DNS blocks a restricted site"
                setShowBadge(false)
            }
            nm.createNotificationChannel(alertChannel)
        }
    }

    private fun notifyDomainBlocked(domain: String) {
        if (!runningFlag.get()) return
        val cleanDomain = domain.removePrefix("www.")
        val now = System.currentTimeMillis()
        val prev = lastBlockNoticeMs[cleanDomain] ?: 0L
        if (now - prev < BLOCK_ALERT_COOLDOWN_MS) return
        lastBlockNoticeMs[cleanDomain] = now

        try {
            val openIntent = Intent(this, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            }
            val pendingOpen = PendingIntent.getActivity(
                this,
                1,
                openIntent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
            val nm = getSystemService(NotificationManager::class.java) ?: return
            val notif = NotificationCompat.Builder(this, BLOCK_ALERT_CHANNEL_ID)
                .setSmallIcon(android.R.drawable.ic_lock_lock)
                .setContentTitle("Blocked $cleanDomain")
                .setContentText("Restricted by your active ONUSLY goal or daily limit.")
                .setAutoCancel(true)
                .setSilent(true)
                .setPriority(NotificationCompat.PRIORITY_LOW)
                .setContentIntent(pendingOpen)
                .build()
            nm.notify(BLOCK_ALERT_NOTIFICATION_ID, notif)
        } catch (_: Exception) {
        }
    }

    private fun buildNotification(domainsCount: Int, queriesBlocked: Long): Notification {
        val openIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val pendingOpen = PendingIntent.getActivity(
            this,
            0,
            openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val statusText = if (domainsCount > 0) {
            "Blocking $domainsCount domain${if (domainsCount == 1) "" else "s"} • $queriesBlocked blocked"
        } else {
            "Active & monitoring • No domains currently restricted"
        }

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setContentTitle("ONUSLY DNS Shield Active")
            .setContentText(statusText)
            .setOngoing(true)
            .setSilent(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setContentIntent(pendingOpen)
            .build()
    }

    private fun startForegroundWithStats(domainsCount: Int, queriesBlocked: Long) {
        val notification = buildNotification(domainsCount, queriesBlocked)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(
                NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
            )
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun updateNotification() {
        try {
            val stats = resolverEngine.getStats()
            val nm = getSystemService(NotificationManager::class.java) ?: return
            nm.notify(NOTIFICATION_ID, buildNotification(stats.blockedDomainsCount, stats.queriesBlocked))
        } catch (_: Exception) {
        }
    }

    private fun stopForegroundCompat() {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
                stopForeground(STOP_FOREGROUND_REMOVE)
            } else {
                @Suppress("DEPRECATION")
                stopForeground(true)
            }
        } catch (_: Exception) {
        }
    }
}
