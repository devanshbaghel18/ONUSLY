package com.onusly.app

import com.onusly.app.data.model.GoalParser
import com.onusly.app.data.model.UploadedFileItem
import com.onusly.app.ui.SubmitProofDialog
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

class FileUploadAndProofTest {

    @Test
    fun testParseUploadedFileList_ArrayResponse() {
        val json = """
            [
                {
                    "id": "file-1234abcd",
                    "name": "homework_proof.png",
                    "size": 245760,
                    "formattedSize": "240 KB",
                    "type": "image/png",
                    "url": "/uploads/proofs/1731584000-abcd1234-homework_proof.png",
                    "dataUrl": "/uploads/proofs/1731584000-abcd1234-homework_proof.png",
                    "isImage": true,
                    "isPdf": false
                }
            ]
        """.trimIndent()

        val list = GoalParser.parseUploadedFileList(json)
        assertEquals(1, list.size)

        val file = list[0]
        assertEquals("file-1234abcd", file.id)
        assertEquals("homework_proof.png", file.name)
        assertEquals(245760L, file.size)
        assertEquals("240 KB", file.formattedSize)
        assertEquals("image/png", file.type)
        assertEquals("/uploads/proofs/1731584000-abcd1234-homework_proof.png", file.url)
        assertTrue(file.isImage)
        assertFalse(file.isPdf)
    }

    @Test
    fun testParseUploadedFileList_SingleObjectFallback() {
        val json = """
            {
                "id": "file-pdf123",
                "name": "study_notes.pdf",
                "size": 1048576,
                "formattedSize": "1.0 MB",
                "type": "application/pdf",
                "url": "/uploads/proofs/study_notes.pdf",
                "isImage": false,
                "isPdf": true
            }
        """.trimIndent()

        val list = GoalParser.parseUploadedFileList(json)
        assertEquals(1, list.size)
        assertEquals("file-pdf123", list[0].id)
        assertEquals("study_notes.pdf", list[0].name)
        assertTrue(list[0].isPdf)
        assertFalse(list[0].isImage)
    }

    @Test
    fun testSubmitProofDialog_IsValidUrl() {
        // Uploaded server relative paths
        assertTrue(SubmitProofDialog.isValidUrl("/uploads/proofs/1731584000-abcd1234-evidence.png"))
        assertTrue(SubmitProofDialog.isValidUrl("/uploads/proofs/test.pdf"))

        // Standard web URLs
        assertTrue(SubmitProofDialog.isValidUrl("https://images.example.com/screenshot.png"))
        assertTrue(SubmitProofDialog.isValidUrl("http://localhost:8081/uploads/proofs/screenshot.png"))

        // Android content and file URIs
        assertTrue(SubmitProofDialog.isValidUrl("content://media/external/images/media/123"))
        assertTrue(SubmitProofDialog.isValidUrl("file:///storage/emulated/0/DCIM/proof.jpg"))

        // Invalid inputs
        assertFalse(SubmitProofDialog.isValidUrl(""))
        assertFalse(SubmitProofDialog.isValidUrl("   "))
        assertFalse(SubmitProofDialog.isValidUrl("ftp://invalid.com/file"))
        assertFalse(SubmitProofDialog.isValidUrl("not a url at all"))
    }
}

