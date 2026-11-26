package com.onusly.app.dns

data class UdpPacket(
    val ipVersion: Int, // 4 or 6
    val srcAddress: ByteArray,
    val dstAddress: ByteArray,
    val srcPort: Int,
    val dstPort: Int,
    val payload: ByteArray
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is UdpPacket) return false
        return ipVersion == other.ipVersion &&
            srcAddress.contentEquals(other.srcAddress) &&
            dstAddress.contentEquals(other.dstAddress) &&
            srcPort == other.srcPort &&
            dstPort == other.dstPort &&
            payload.contentEquals(other.payload)
    }

    override fun hashCode(): Int {
        var result = ipVersion
        result = 31 * result + srcAddress.contentHashCode()
        result = 31 * result + dstAddress.contentHashCode()
        result = 31 * result + srcPort
        result = 31 * result + dstPort
        result = 31 * result + payload.contentHashCode()
        return result
    }
}

/**
 * Pure-Kotlin IPv4/IPv6 + UDP packet codec for Android TUN interface frames.
 * Parses incoming TUN IP datagrams and constructs checksum-verified UDP response packets.
 */
object IpUdpPacketCodec {

    private const val PROTOCOL_UDP = 17

    /**
     * Parses a raw IPv4 or IPv6 UDP packet from the TUN interface.
     * Returns null for non-UDP packets or malformed frames.
     */
    fun parseUdpPacket(raw: ByteArray, length: Int = raw.size): UdpPacket? {
        if (length < 20 || length > raw.size) return null
        val version = (raw[0].toInt() ushr 4) and 0x0F

        return when (version) {
            4 -> parseIpv4Udp(raw, length)
            6 -> parseIpv6Udp(raw, length)
            else -> null
        }
    }

    private fun parseIpv4Udp(raw: ByteArray, length: Int): UdpPacket? {
        val ihl = (raw[0].toInt() and 0x0F) * 4
        if (ihl < 20 || length < ihl + 8) return null

        // Drop fragmented IPv4 packets safely to avoid parsing incomplete UDP/DNS frames
        val fragOffset = ((raw[6].toInt() and 0x1F) shl 8) or (raw[7].toInt() and 0xFF)
        val moreFrags = (raw[6].toInt() and 0x20) != 0
        if (moreFrags || fragOffset != 0) return null

        val protocol = raw[9].toInt() and 0xFF
        if (protocol != PROTOCOL_UDP) return null

        val totalLen = ((raw[2].toInt() and 0xFF) shl 8) or (raw[3].toInt() and 0xFF)
        val effectiveLen = minOf(length, totalLen)
        if (effectiveLen < ihl + 8) return null

        val srcIp = raw.copyOfRange(12, 16)
        val dstIp = raw.copyOfRange(16, 20)

        val srcPort = ((raw[ihl].toInt() and 0xFF) shl 8) or (raw[ihl + 1].toInt() and 0xFF)
        val dstPort = ((raw[ihl + 2].toInt() and 0xFF) shl 8) or (raw[ihl + 3].toInt() and 0xFF)
        val udpLen = ((raw[ihl + 4].toInt() and 0xFF) shl 8) or (raw[ihl + 5].toInt() and 0xFF)

        if (udpLen < 8 || ihl + udpLen > effectiveLen) return null
        val payload = raw.copyOfRange(ihl + 8, ihl + udpLen)

        return UdpPacket(
            ipVersion = 4,
            srcAddress = srcIp,
            dstAddress = dstIp,
            srcPort = srcPort,
            dstPort = dstPort,
            payload = payload
        )
    }

    private fun parseIpv6Udp(raw: ByteArray, length: Int): UdpPacket? {
        if (length < 48) return null // 40 bytes IPv6 header + 8 bytes UDP header

        val nextHeader = raw[6].toInt() and 0xFF
        if (nextHeader != PROTOCOL_UDP) return null

        val payloadLength = ((raw[4].toInt() and 0xFF) shl 8) or (raw[5].toInt() and 0xFF)
        val effectiveLen = minOf(length, 40 + payloadLength)
        if (effectiveLen < 48) return null

        val srcIp = raw.copyOfRange(8, 24)
        val dstIp = raw.copyOfRange(24, 40)

        val udpStart = 40
        val srcPort = ((raw[udpStart].toInt() and 0xFF) shl 8) or (raw[udpStart + 1].toInt() and 0xFF)
        val dstPort = ((raw[udpStart + 2].toInt() and 0xFF) shl 8) or (raw[udpStart + 3].toInt() and 0xFF)
        val udpLen = ((raw[udpStart + 4].toInt() and 0xFF) shl 8) or (raw[udpStart + 5].toInt() and 0xFF)

        if (udpLen < 8 || udpStart + udpLen > effectiveLen) return null
        val payload = raw.copyOfRange(udpStart + 8, udpStart + udpLen)

        return UdpPacket(
            ipVersion = 6,
            srcAddress = srcIp,
            dstAddress = dstIp,
            srcPort = srcPort,
            dstPort = dstPort,
            payload = payload
        )
    }

    /**
     * Builds a UDP response IP packet swapping src/dst from [requestPacket] and setting [responsePayload].
     */
    fun buildUdpResponse(requestPacket: UdpPacket, responsePayload: ByteArray): ByteArray {
        return when (requestPacket.ipVersion) {
            4 -> buildIpv4UdpResponse(
                srcIp = requestPacket.dstAddress,
                dstIp = requestPacket.srcAddress,
                srcPort = requestPacket.dstPort,
                dstPort = requestPacket.srcPort,
                payload = responsePayload
            )
            6 -> buildIpv6UdpResponse(
                srcIp = requestPacket.dstAddress,
                dstIp = requestPacket.srcAddress,
                srcPort = requestPacket.dstPort,
                dstPort = requestPacket.srcPort,
                payload = responsePayload
            )
            else -> throw IllegalArgumentException("Unsupported IP version: ${requestPacket.ipVersion}")
        }
    }

    private fun buildIpv4UdpResponse(
        srcIp: ByteArray,
        dstIp: ByteArray,
        srcPort: Int,
        dstPort: Int,
        payload: ByteArray
    ): ByteArray {
        require(srcIp.size == 4 && dstIp.size == 4)
        val udpLen = 8 + payload.size
        val totalLen = 20 + udpLen
        val pkt = ByteArray(totalLen)

        // IPv4 header (20 bytes)
        pkt[0] = 0x45.toByte() // Version 4, IHL 5 (20 bytes)
        pkt[1] = 0x00 // TOS
        pkt[2] = ((totalLen ushr 8) and 0xFF).toByte()
        pkt[3] = (totalLen and 0xFF).toByte()
        pkt[4] = 0x00 // Identification
        pkt[5] = 0x01
        pkt[6] = 0x40 // Don't Fragment
        pkt[7] = 0x00
        pkt[8] = 64 // TTL
        pkt[9] = PROTOCOL_UDP.toByte()
        pkt[10] = 0x00 // Checksum placeholder
        pkt[11] = 0x00
        System.arraycopy(srcIp, 0, pkt, 12, 4)
        System.arraycopy(dstIp, 0, pkt, 16, 4)

        val ipChecksum = computeChecksum(pkt, 0, 20)
        pkt[10] = ((ipChecksum ushr 8) and 0xFF).toByte()
        pkt[11] = (ipChecksum and 0xFF).toByte()

        // UDP header (8 bytes at offset 20)
        pkt[20] = ((srcPort ushr 8) and 0xFF).toByte()
        pkt[21] = (srcPort and 0xFF).toByte()
        pkt[22] = ((dstPort ushr 8) and 0xFF).toByte()
        pkt[23] = (dstPort and 0xFF).toByte()
        pkt[24] = ((udpLen ushr 8) and 0xFF).toByte()
        pkt[25] = (udpLen and 0xFF).toByte()
        pkt[26] = 0x00
        pkt[27] = 0x00

        System.arraycopy(payload, 0, pkt, 28, payload.size)

        val udpChecksum = computeIpv4UdpChecksum(srcIp, dstIp, pkt, 20, udpLen)
        pkt[26] = ((udpChecksum ushr 8) and 0xFF).toByte()
        pkt[27] = (udpChecksum and 0xFF).toByte()

        return pkt
    }

    private fun buildIpv6UdpResponse(
        srcIp: ByteArray,
        dstIp: ByteArray,
        srcPort: Int,
        dstPort: Int,
        payload: ByteArray
    ): ByteArray {
        require(srcIp.size == 16 && dstIp.size == 16)
        val udpLen = 8 + payload.size
        val totalLen = 40 + udpLen
        val pkt = ByteArray(totalLen)

        // IPv6 header (40 bytes)
        pkt[0] = 0x60.toByte() // Version 6
        pkt[1] = 0x00
        pkt[2] = 0x00
        pkt[3] = 0x00
        pkt[4] = ((udpLen ushr 8) and 0xFF).toByte()
        pkt[5] = (udpLen and 0xFF).toByte()
        pkt[6] = PROTOCOL_UDP.toByte() // Next Header = UDP
        pkt[7] = 64 // Hop Limit
        System.arraycopy(srcIp, 0, pkt, 8, 16)
        System.arraycopy(dstIp, 0, pkt, 24, 16)

        // UDP header (8 bytes at offset 40)
        pkt[40] = ((srcPort ushr 8) and 0xFF).toByte()
        pkt[41] = (srcPort and 0xFF).toByte()
        pkt[42] = ((dstPort ushr 8) and 0xFF).toByte()
        pkt[43] = (dstPort and 0xFF).toByte()
        pkt[44] = ((udpLen ushr 8) and 0xFF).toByte()
        pkt[45] = (udpLen and 0xFF).toByte()
        pkt[46] = 0x00
        pkt[47] = 0x00

        System.arraycopy(payload, 0, pkt, 48, payload.size)

        val udpChecksum = computeIpv6UdpChecksum(srcIp, dstIp, pkt, 40, udpLen)
        pkt[46] = ((udpChecksum ushr 8) and 0xFF).toByte()
        pkt[47] = (udpChecksum and 0xFF).toByte()

        return pkt
    }

    private fun computeChecksum(data: ByteArray, offset: Int, length: Int): Int {
        var sum = accumulate16BitWords(data, offset, length)
        return finalizeOnesComplement(sum)
    }

    private fun computeIpv4UdpChecksum(
        srcIp: ByteArray,
        dstIp: ByteArray,
        udpSegment: ByteArray,
        udpOffset: Int,
        udpLen: Int
    ): Int {
        var sum = 0L
        sum += accumulate16BitWords(srcIp, 0, 4)
        sum += accumulate16BitWords(dstIp, 0, 4)
        sum += PROTOCOL_UDP.toLong()
        sum += udpLen.toLong()
        sum += accumulate16BitWords(udpSegment, udpOffset, udpLen)
        val csum = finalizeOnesComplement(sum)
        return if (csum == 0) 0xFFFF else csum
    }

    private fun computeIpv6UdpChecksum(
        srcIp: ByteArray,
        dstIp: ByteArray,
        udpSegment: ByteArray,
        udpOffset: Int,
        udpLen: Int
    ): Int {
        var sum = 0L
        sum += accumulate16BitWords(srcIp, 0, 16)
        sum += accumulate16BitWords(dstIp, 0, 16)
        sum += ((udpLen ushr 16) and 0xFFFF).toLong()
        sum += (udpLen and 0xFFFF).toLong()
        sum += PROTOCOL_UDP.toLong()
        sum += accumulate16BitWords(udpSegment, udpOffset, udpLen)
        val csum = finalizeOnesComplement(sum)
        return if (csum == 0) 0xFFFF else csum
    }

    private fun accumulate16BitWords(data: ByteArray, offset: Int, length: Int): Long {
        var sum = 0L
        var i = offset
        val end = offset + length
        while (i + 1 < end) {
            val word = ((data[i].toInt() and 0xFF) shl 8) or (data[i + 1].toInt() and 0xFF)
            sum += word.toLong()
            i += 2
        }
        if (i < end) {
            sum += ((data[i].toInt() and 0xFF) shl 8).toLong()
        }
        return sum
    }

    private fun finalizeOnesComplement(initialSum: Long): Int {
        var sum = initialSum
        while ((sum ushr 16) != 0L) {
            sum = (sum and 0xFFFFL) + (sum ushr 16)
        }
        return (sum.inv() and 0xFFFFL).toInt()
    }
}
