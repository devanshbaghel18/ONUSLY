package com.onusly.app

import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.dns.DnsResolverEngine
import com.onusly.app.dns.DnsResponseCache
import com.onusly.app.dns.DnsWireCodec
import com.onusly.app.dns.IpUdpPacketCodec
import com.onusly.app.dns.UdpPacket
import com.onusly.app.dns.UpstreamDnsTransport
import com.onusly.app.engine.DomainMatcher
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.nio.charset.StandardCharsets

class MobileDnsBlockingTest {

    private fun buildSampleDnsQuery(
        txId: Int,
        domain: String,
        qType: Int = DnsWireCodec.TYPE_A,
        qClass: Int = 1,
        rd: Boolean = true
    ): ByteArray {
        val out = ByteArrayOutputStream()
        out.write((txId ushr 8) and 0xFF)
        out.write(txId and 0xFF)
        val flags = if (rd) 0x0100 else 0x0000
        out.write((flags ushr 8) and 0xFF)
        out.write(flags and 0xFF)
        // QDCOUNT = 1, AN = 0, NS = 0, AR = 0
        out.write(0); out.write(1)
        out.write(0); out.write(0)
        out.write(0); out.write(0)
        out.write(0); out.write(0)
        for (label in domain.trim('.').split('.')) {
            val bytes = label.toByteArray(StandardCharsets.US_ASCII)
            out.write(bytes.size)
            out.write(bytes)
        }
        out.write(0)
        out.write((qType ushr 8) and 0xFF)
        out.write(qType and 0xFF)
        out.write((qClass ushr 8) and 0xFF)
        out.write(qClass and 0xFF)
        return out.toByteArray()
    }

    private fun buildSampleDnsNoErrorResponse(rawQuery: ByteArray, ipBytes: ByteArray = byteArrayOf(1, 2, 3, 4)): ByteArray {
        val out = ByteArrayOutputStream()
        // Copy txId + flags (set QR=1, RA=1, RCODE=0)
        out.write(rawQuery[0].toInt() and 0xFF)
        out.write(rawQuery[1].toInt() and 0xFF)
        out.write(0x81); out.write(0x80)
        // QD=1, AN=1, NS=0, AR=0
        out.write(0); out.write(1)
        out.write(0); out.write(1)
        out.write(0); out.write(0)
        out.write(0); out.write(0)
        // Copy question
        out.write(rawQuery, 12, rawQuery.size - 12)
        // Answer pointing to offset 12 (0xC00C)
        out.write(0xC0); out.write(0x0C)
        out.write(0); out.write(1) // A
        out.write(0); out.write(1) // IN
        out.write(0); out.write(0); out.write(0); out.write(60) // TTL 60s
        out.write(0); out.write(ipBytes.size)
        out.write(ipBytes)
        return out.toByteArray()
    }

    private fun buildIpv4UdpRaw(
        srcIp: ByteArray,
        dstIp: ByteArray,
        srcPort: Int,
        dstPort: Int,
        payload: ByteArray
    ): ByteArray {
        val req = UdpPacket(
            ipVersion = 4,
            srcAddress = dstIp,
            dstAddress = srcIp,
            srcPort = dstPort,
            dstPort = srcPort,
            payload = ByteArray(0)
        )
        return IpUdpPacketCodec.buildUdpResponse(req, payload)
    }

    private fun buildIpv6UdpRaw(
        srcIp: ByteArray,
        dstIp: ByteArray,
        srcPort: Int,
        dstPort: Int,
        payload: ByteArray
    ): ByteArray {
        val req = UdpPacket(
            ipVersion = 6,
            srcAddress = dstIp,
            dstAddress = srcIp,
            srcPort = dstPort,
            dstPort = srcPort,
            payload = ByteArray(0)
        )
        return IpUdpPacketCodec.buildUdpResponse(req, payload)
    }

    // --- 1. DomainMatcher Tests ---

    @Test
    fun testDomainMatching_ExactSubdomainAndWwwParity() {
        assertTrue(DomainMatcher.isDomainMatch("instagram.com", "instagram.com"))
        assertTrue(DomainMatcher.isDomainMatch("www.instagram.com", "instagram.com"))
        assertTrue(DomainMatcher.isDomainMatch("instagram.com", "www.instagram.com"))
        assertTrue(DomainMatcher.isDomainMatch("m.instagram.com", "instagram.com"))
        assertTrue(DomainMatcher.isDomainMatch("shorts.youtube.com", "https://youtube.com/shorts"))

        // Prevent unsafe substring collisions
        assertFalse(DomainMatcher.isDomainMatch("notinstagram.com", "instagram.com"))
        assertFalse(DomainMatcher.isDomainMatch("instagram.com.evil.org", "instagram.com"))
        assertFalse(DomainMatcher.isDomainMatch("my-instagram.com", "instagram.com"))
    }

    @Test
    fun testProtectedDomains_NeverBlockedEvenIfInTargets() {
        val blocklist = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-11-26T10:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "g1",
                    goalTitle = "Bad Targets",
                    status = "active",
                    apps = emptyList(),
                    domains = listOf("localhost", "api.onusly.com", "custom.myserver.dev", "instagram.com")
                )
            )
        )

        val extraProtected = setOf("custom.myserver.dev")
        assertFalse(DomainMatcher.evaluateDomain("localhost", blocklist, extraProtected).isBlocked)
        assertFalse(DomainMatcher.evaluateDomain("api.onusly.com", blocklist, extraProtected).isBlocked)
        assertFalse(DomainMatcher.evaluateDomain("onusly.app", blocklist, extraProtected).isBlocked)
        assertFalse(DomainMatcher.evaluateDomain("custom.myserver.dev", blocklist, extraProtected).isBlocked)
        assertTrue(DomainMatcher.evaluateDomain("www.instagram.com", blocklist, extraProtected).isBlocked)
    }

    @Test
    fun testGoalStatuses_RespectEnforcingLifecycle() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-2",
            generatedAt = "2026-11-26T10:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Active Goal", "active", emptyList(), listOf("x.com")),
                BlocklistTarget("g2", "Pending Proof", "proof_submitted", emptyList(), listOf("reddit.com")),
                BlocklistTarget("g3", "Rejected Proof", "rejected", emptyList(), listOf("tiktok.com")),
                BlocklistTarget("g4", "Completed Goal", "completed", emptyList(), listOf("wikipedia.org"))
            )
        )

        assertTrue(DomainMatcher.evaluateDomain("x.com", cache).isBlocked)
        assertTrue(DomainMatcher.evaluateDomain("old.reddit.com", cache).isBlocked)
        assertTrue(DomainMatcher.evaluateDomain("tiktok.com", cache).isBlocked)
        assertFalse(DomainMatcher.evaluateDomain("en.wikipedia.org", cache).isBlocked)
    }

    // --- 2. DnsWireCodec Tests ---

    @Test
    fun testDnsWireCodec_ParseAndSynthesizeNxDomain() {
        val rawQuery = buildSampleDnsQuery(txId = 0x4A2F, domain = "www.instagram.com", qType = DnsWireCodec.TYPE_AAAA)
        val question = DnsWireCodec.parseQuery(rawQuery)
        assertNotNull(question)
        assertEquals(0x4A2F, question!!.transactionId)
        assertEquals("www.instagram.com", question.qName)
        assertEquals(DnsWireCodec.TYPE_AAAA, question.qType)
        assertTrue(question.recursionDesired)

        val nxResp = DnsWireCodec.buildNxDomainResponse(rawQuery, question)
        assertEquals(0x4A, nxResp[0].toInt() and 0xFF)
        assertEquals(0x2F, nxResp[1].toInt() and 0xFF)
        assertEquals(DnsWireCodec.RCODE_NXDOMAIN, DnsWireCodec.extractRCode(nxResp))
        // Ensure QR=1 and AD=0
        val respFlags = ((nxResp[2].toInt() and 0xFF) shl 8) or (nxResp[3].toInt() and 0xFF)
        assertTrue((respFlags and 0x8000) != 0) // QR=1
        assertTrue((respFlags and 0x0020) == 0) // AD=0
    }

    @Test
    fun testDnsWireCodec_CyclicCompressionPointer_RejectedSafely() {
        // Malformed query with self-referencing compression pointer at offset 12 (0xC0 0x0C)
        val malformed = byteArrayOf(
            0x12, 0x34, 0x01, 0x00,
            0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0xC0.toByte(), 0x0C,
            0x00, 0x01, 0x00, 0x01
        )
        assertNull(DnsWireCodec.parseQuery(malformed))
    }

    // --- 3. IpUdpPacketCodec Tests ---

    @Test
    fun testIpUdpPacketCodec_Ipv4AndIpv6RoundTrip() {
        val dnsPayload = buildSampleDnsQuery(0x1122, "google.com")
        val clientV4 = byteArrayOf(10, 111, 222.toByte(), 1)
        val dnsV4 = byteArrayOf(10, 111, 222.toByte(), 2)

        val rawV4 = buildIpv4UdpRaw(clientV4, dnsV4, srcPort = 54321, dstPort = 53, payload = dnsPayload)
        val parsedV4 = IpUdpPacketCodec.parseUdpPacket(rawV4)
        assertNotNull(parsedV4)
        assertEquals(4, parsedV4!!.ipVersion)
        assertArrayEquals(clientV4, parsedV4.srcAddress)
        assertArrayEquals(dnsV4, parsedV4.dstAddress)
        assertEquals(54321, parsedV4.srcPort)
        assertEquals(53, parsedV4.dstPort)
        assertArrayEquals(dnsPayload, parsedV4.payload)

        val respPayload = DnsWireCodec.buildServFailResponse(dnsPayload)
        val respV4 = IpUdpPacketCodec.buildUdpResponse(parsedV4, respPayload)
        val parsedRespV4 = IpUdpPacketCodec.parseUdpPacket(respV4)
        assertNotNull(parsedRespV4)
        assertArrayEquals(dnsV4, parsedRespV4!!.srcAddress)
        assertArrayEquals(clientV4, parsedRespV4.dstAddress)
        assertEquals(53, parsedRespV4.srcPort)
        assertEquals(54321, parsedRespV4.dstPort)
        assertArrayEquals(respPayload, parsedRespV4.payload)

        // IPv6 check
        val clientV6 = ByteArray(16).apply { this[0] = 0xFD.toByte(); this[15] = 1 }
        val dnsV6 = ByteArray(16).apply { this[0] = 0xFD.toByte(); this[15] = 2 }
        val rawV6 = buildIpv6UdpRaw(clientV6, dnsV6, srcPort = 40000, dstPort = 53, payload = dnsPayload)
        val parsedV6 = IpUdpPacketCodec.parseUdpPacket(rawV6)
        assertNotNull(parsedV6)
        assertEquals(6, parsedV6!!.ipVersion)
        assertArrayEquals(clientV6, parsedV6.srcAddress)
        assertArrayEquals(dnsV6, parsedV6.dstAddress)
        assertArrayEquals(dnsPayload, parsedV6.payload)
    }

    // --- 4. DnsResolverEngine Integration Tests ---

    @Test
    fun testResolverEngine_BlocksPolicyDomainsCachesAllowedAndInvalidatesOnUpdate() {
        var upstreamCalls = 0
        val fakeUpstream = object : UpstreamDnsTransport {
            override fun exchange(rawQuery: ByteArray): ByteArray {
                upstreamCalls++
                return buildSampleDnsNoErrorResponse(rawQuery)
            }
        }

        val engine = DnsResolverEngine(upstream = fakeUpstream, cache = DnsResponseCache())

        // Initially empty policy -> google.com and instagram.com both forward
        val qGoogle1 = buildSampleDnsQuery(0x0001, "google.com")
        val rGoogle1 = engine.handleDnsQuery(qGoogle1)
        assertEquals(DnsWireCodec.RCODE_NOERROR, DnsWireCodec.extractRCode(rGoogle1))
        assertEquals(1, upstreamCalls)

        // Second query hits cache with updated Transaction ID
        val qGoogle2 = buildSampleDnsQuery(0x0002, "google.com")
        val rGoogle2 = engine.handleDnsQuery(qGoogle2)
        assertEquals(DnsWireCodec.RCODE_NOERROR, DnsWireCodec.extractRCode(rGoogle2))
        assertEquals(0x00, rGoogle2[0].toInt() and 0xFF)
        assertEquals(0x02, rGoogle2[1].toInt() and 0xFF)
        assertEquals(1, upstreamCalls) // Still 1!

        // Query instagram.com -> forwarded & cached
        val qInsta1 = buildSampleDnsQuery(0x0003, "www.instagram.com")
        val rInsta1 = engine.handleDnsQuery(qInsta1)
        assertEquals(DnsWireCodec.RCODE_NOERROR, DnsWireCodec.extractRCode(rInsta1))
        assertEquals(2, upstreamCalls)

        // Now activate a goal blocking instagram.com
        engine.updatePolicy(
            CachedBlocklist(
                userId = "u1",
                version = "v1-active",
                generatedAt = "2026-11-26T10:05:00Z",
                lastSyncTimestamp = 2000L,
                targets = listOf(
                    BlocklistTarget("g1", "Study", "active", emptyList(), listOf("instagram.com"))
                )
            )
        )

        // Cached entry must be invalidated & blocked immediately with NXDOMAIN!
        val qInsta2 = buildSampleDnsQuery(0x0004, "www.instagram.com")
        val rInsta2 = engine.handleDnsQuery(qInsta2)
        assertEquals(DnsWireCodec.RCODE_NXDOMAIN, DnsWireCodec.extractRCode(rInsta2))
        assertEquals(2, upstreamCalls)

        // Stats and Digital Wellbeing hit counter check
        val stats = engine.getStats()
        assertEquals(4L, stats.queriesTotal)
        assertEquals(1L, stats.queriesBlocked)
        assertEquals(2L, stats.queriesForwarded)
        assertEquals(1L, stats.queriesCached)
        assertEquals("www.instagram.com", stats.lastBlockedDomain)
        assertEquals(1, stats.blockedDomainsCount)

        val hits = engine.getRecentDomainHits()
        val instaHit = hits.find { it.domain == "instagram.com" }
        assertNotNull(instaHit)
        assertEquals(2L, instaHit!!.count)
        assertEquals(1L, instaHit.opens)
        assertEquals(1L, instaHit.blockedAttempts)
        assertTrue(instaHit.blocked)

        // When goal completes -> immediately unblocked
        engine.updatePolicy(
            CachedBlocklist(
                userId = "u1",
                version = "v1-done",
                generatedAt = "2026-11-26T10:10:00Z",
                lastSyncTimestamp = 3000L,
                targets = listOf(
                    BlocklistTarget("g1", "Study", "completed", emptyList(), listOf("instagram.com"))
                )
            )
        )

        val qInsta3 = buildSampleDnsQuery(0x0005, "www.instagram.com")
        val rInsta3 = engine.handleDnsQuery(qInsta3)
        assertEquals(DnsWireCodec.RCODE_NOERROR, DnsWireCodec.extractRCode(rInsta3))
        assertEquals(3, upstreamCalls)
    }
}
