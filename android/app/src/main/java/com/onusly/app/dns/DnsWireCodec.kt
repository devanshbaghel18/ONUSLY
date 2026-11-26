package com.onusly.app.dns

import java.io.ByteArrayOutputStream
import java.nio.charset.StandardCharsets

data class DnsQuestion(
    val transactionId: Int,
    val flags: Int,
    val opCode: Int,
    val recursionDesired: Boolean,
    val qName: String,
    val qType: Int,
    val qClass: Int,
    val questionEndOffset: Int
)

/**
 * Pure-Kotlin RFC 1035 DNS wire parser & synthesizer.
 * Handles question decoding (including pointer compression with cycle protection)
 * and authoritative NXDOMAIN / SERVFAIL response construction.
 */
object DnsWireCodec {

    const val TYPE_A = 1
    const val TYPE_CNAME = 5
    const val TYPE_AAAA = 28
    const val TYPE_SVCB = 64
    const val TYPE_HTTPS = 65

    const val RCODE_NOERROR = 0
    const val RCODE_SERVFAIL = 2
    const val RCODE_NXDOMAIN = 3

    /**
     * Parses the first DNS Question from a raw DNS UDP payload.
     * Returns null if malformed, truncated, or if packet is a response rather than a query.
     */
    fun parseQuery(raw: ByteArray): DnsQuestion? {
        if (raw.size < 12) return null

        val txId = ((raw[0].toInt() and 0xFF) shl 8) or (raw[1].toInt() and 0xFF)
        val flags = ((raw[2].toInt() and 0xFF) shl 8) or (raw[3].toInt() and 0xFF)

        // QR bit (bit 15): 0 = Query, 1 = Response
        val isResponse = (flags and 0x8000) != 0
        if (isResponse) return null

        val opCode = (flags ushr 11) and 0x0F
        val rd = (flags and 0x0100) != 0

        val qdCount = ((raw[4].toInt() and 0xFF) shl 8) or (raw[5].toInt() and 0xFF)
        if (qdCount < 1) return null

        val decoded = decodeName(raw, 12) ?: return null
        val (qName, nextOffset) = decoded

        if (nextOffset + 4 > raw.size) return null
        val qType = ((raw[nextOffset].toInt() and 0xFF) shl 8) or (raw[nextOffset + 1].toInt() and 0xFF)
        val qClass = ((raw[nextOffset + 2].toInt() and 0xFF) shl 8) or (raw[nextOffset + 3].toInt() and 0xFF)

        return DnsQuestion(
            transactionId = txId,
            flags = flags,
            opCode = opCode,
            recursionDesired = rd,
            qName = qName,
            qType = qType,
            qClass = qClass,
            questionEndOffset = nextOffset + 4
        )
    }

    /**
     * Decodes a DNS domain name starting at [startOffset], following compression pointers safely.
     * Returns (hostname, offsetImmediatelyAfterUncompressedName).
     */
    fun decodeName(raw: ByteArray, startOffset: Int): Pair<String, Int>? {
        if (startOffset < 0 || startOffset >= raw.size) return null

        val labels = mutableListOf<String>()
        var offset = startOffset
        var endOffset = -1
        var jumped = false
        var jumps = 0

        while (true) {
            if (offset >= raw.size) return null
            val len = raw[offset].toInt() and 0xFF

            if (len == 0) {
                if (!jumped) {
                    endOffset = offset + 1
                }
                break
            }

            if ((len and 0xC0) == 0xC0) {
                // Compression pointer (2 bytes)
                if (offset + 1 >= raw.size) return null
                if (!jumped) {
                    endOffset = offset + 2
                }
                val ptr = ((len and 0x3F) shl 8) or (raw[offset + 1].toInt() and 0xFF)
                if (ptr >= raw.size || ptr == offset) return null
                jumps++
                if (jumps > 16) return null // Prevent cyclic compression DoS
                offset = ptr
                jumped = true
                continue
            }

            // Standard label cannot exceed 63 bytes per RFC 1035
            if (len > 63 || offset + 1 + len > raw.size) return null
            val label = String(raw, offset + 1, len, StandardCharsets.US_ASCII)
            labels.add(label)
            offset += 1 + len
        }

        if (endOffset == -1) return null
        return Pair(labels.joinToString("."), endOffset)
    }

    /**
     * Encodes an authoritative NXDOMAIN response matching [query] and copying its Question section.
     * Matches desktop agent invariant: never sets AD (Authenticated Data) bit.
     */
    fun buildNxDomainResponse(rawQuery: ByteArray, question: DnsQuestion): ByteArray {
        val out = ByteArrayOutputStream(question.questionEndOffset)

        // Header: 12 bytes
        out.write((question.transactionId ushr 8) and 0xFF)
        out.write(question.transactionId and 0xFF)

        // Flags: QR=1, Opcode=query, AA=1, TC=0, RD=query.RD, RA=1, Z=0, AD=0, CD=0, RCODE=3 (NXDOMAIN)
        var respFlags = 0x8480 or RCODE_NXDOMAIN // QR(0x8000) | AA(0x0400) | RA(0x0080) | RCODE(3)
        respFlags = respFlags or ((question.opCode and 0x0F) shl 11)
        if (question.recursionDesired) {
            respFlags = respFlags or 0x0100
        }
        out.write((respFlags ushr 8) and 0xFF)
        out.write(respFlags and 0xFF)

        // QDCOUNT = 1, ANCOUNT = 0, NSCOUNT = 0, ARCOUNT = 0
        out.write(0); out.write(1)
        out.write(0); out.write(0)
        out.write(0); out.write(0)
        out.write(0); out.write(0)

        // Copy raw Question bytes (from offset 12 to questionEndOffset)
        if (question.questionEndOffset <= rawQuery.size && question.questionEndOffset > 12) {
            out.write(rawQuery, 12, question.questionEndOffset - 12)
        } else {
            encodeName(out, question.qName)
            out.write((question.qType ushr 8) and 0xFF)
            out.write(question.qType and 0xFF)
            out.write((question.qClass ushr 8) and 0xFF)
            out.write(question.qClass and 0xFF)
        }

        return out.toByteArray()
    }

    /**
     * Builds a SERVFAIL fallback response when upstream resolution fails.
     */
    fun buildServFailResponse(rawQuery: ByteArray): ByteArray {
        val txId = if (rawQuery.size >= 2) {
            ((rawQuery[0].toInt() and 0xFF) shl 8) or (rawQuery[1].toInt() and 0xFF)
        } else {
            0
        }
        val resp = ByteArray(12)
        resp[0] = ((txId ushr 8) and 0xFF).toByte()
        resp[1] = (txId and 0xFF).toByte()
        // QR=1, RA=1, RCODE=2 (SERVFAIL) -> 0x8082
        resp[2] = 0x80.toByte()
        resp[3] = 0x82.toByte()
        return resp
    }

    /**
     * Rewrites the 2-byte Transaction ID at the start of a cached DNS response.
     */
    fun rewriteTransactionId(response: ByteArray, transactionId: Int): ByteArray {
        val copy = response.copyOf()
        if (copy.size >= 2) {
            copy[0] = ((transactionId ushr 8) and 0xFF).toByte()
            copy[1] = (transactionId and 0xFF).toByte()
        }
        return copy
    }

    /**
     * Extracts the RCode (0..15) from a DNS response packet, or -1 if malformed.
     */
    fun extractRCode(rawResponse: ByteArray): Int {
        if (rawResponse.size < 4) return -1
        return rawResponse[3].toInt() and 0x0F
    }

    private fun encodeName(out: ByteArrayOutputStream, domain: String) {
        val clean = domain.trim('.')
        if (clean.isEmpty()) {
            out.write(0)
            return
        }
        for (label in clean.split('.')) {
            val bytes = label.toByteArray(StandardCharsets.US_ASCII)
            val len = bytes.size.coerceAtMost(63)
            out.write(len)
            out.write(bytes, 0, len)
        }
        out.write(0)
    }
}
