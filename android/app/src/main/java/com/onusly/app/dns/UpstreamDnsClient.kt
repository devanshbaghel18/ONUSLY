package com.onusly.app.dns

import java.io.DataInputStream
import java.io.DataOutputStream
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Socket

interface UpstreamDnsTransport {
    fun exchange(rawQuery: ByteArray): ByteArray?
}

/**
 * Upstream DNS forwarder using IP-only public resolvers with UDP-first and automatic TCP fallback.
 * Uses [protectDatagram] / [protectSocket] (VpnService.protect) so upstream sockets bypass the VPN tunnel.
 */
class UpstreamDnsClient(
    private val upstreamIps: List<String> = DEFAULT_UPSTREAMS,
    private val timeoutMs: Int = 2500,
    private val protectDatagram: (DatagramSocket) -> Boolean = { true },
    private val protectSocket: (Socket) -> Boolean = { true }
) : UpstreamDnsTransport {

    companion object {
        val DEFAULT_UPSTREAMS = listOf(
            "1.1.1.1",
            "8.8.8.8",
            "9.9.9.9",
            "2606:4700:4700::1111",
            "2001:4860:4860::8888"
        )
        private const val DNS_PORT = 53
        private const val MAX_UDP_DNS_SIZE = 4096
    }

    override fun exchange(rawQuery: ByteArray): ByteArray? {
        if (rawQuery.size < 12) return null

        for (ip in upstreamIps) {
            val udpResp = tryUdp(ip, rawQuery)
            if (udpResp != null) {
                val isTruncated = udpResp.size >= 3 && ((udpResp[2].toInt() and 0x02) != 0)
                if (!isTruncated) {
                    return udpResp
                }
            }

            // Automatic TCP fallback if UDP response was truncated (TC=1) or failed
            val tcpResp = tryTcp(ip, rawQuery)
            if (tcpResp != null) {
                return tcpResp
            }
        }

        return null
    }

    private fun tryUdp(ip: String, rawQuery: ByteArray): ByteArray? {
        var socket: DatagramSocket? = null
        return try {
            socket = DatagramSocket()
            if (!protectDatagram(socket)) {
                return null
            }
            socket.soTimeout = timeoutMs

            val addr = InetAddress.getByName(ip)
            val sendPacket = DatagramPacket(rawQuery, rawQuery.size, addr, DNS_PORT)
            socket.send(sendPacket)

            val buf = ByteArray(MAX_UDP_DNS_SIZE)
            val recvPacket = DatagramPacket(buf, buf.size)
            socket.receive(recvPacket)
            if (recvPacket.length >= 12) {
                buf.copyOfRange(0, recvPacket.length)
            } else {
                null
            }
        } catch (_: Exception) {
            null
        } finally {
            try {
                socket?.close()
            } catch (_: Exception) {
            }
        }
    }

    private fun tryTcp(ip: String, rawQuery: ByteArray): ByteArray? {
        var socket: Socket? = null
        return try {
            socket = Socket()
            if (!protectSocket(socket)) {
                return null
            }
            socket.soTimeout = timeoutMs
            socket.connect(InetSocketAddress(InetAddress.getByName(ip), DNS_PORT), timeoutMs)

            val out = DataOutputStream(socket.getOutputStream())
            out.writeShort(rawQuery.size)
            out.write(rawQuery)
            out.flush()

            val inp = DataInputStream(socket.getInputStream())
            val respLen = inp.readUnsignedShort()
            if (respLen < 12 || respLen > 65535) {
                return null
            }
            val respBuf = ByteArray(respLen)
            inp.readFully(respBuf)
            respBuf
        } catch (_: Exception) {
            null
        } finally {
            try {
                socket?.close()
            } catch (_: Exception) {
            }
        }
    }
}
