package com.onusly.app

import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.model.SyncResult
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class ApiClientTest {

    private lateinit var server: MockWebServer
    private lateinit var client: EnforcementApiClient

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        client = EnforcementApiClient()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun testFetchBlocklist_200Success() = runBlocking {
        val responseBody = """
            {
              "userId": "user_api_test",
              "version": "v1-0123456789abcdef",
              "generatedAt": "2026-10-08T12:00:00Z",
              "targets": [
                {
                  "goalId": "goal_1",
                  "goalTitle": "Workout",
                  "status": "active",
                  "apps": ["com.instagram.android"],
                  "domains": ["instagram.com"]
                }
              ]
            }
        """.trimIndent()

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setHeader("ETag", "\"v1-0123456789abcdef\"")
                .setBody(responseBody)
        )

        val baseUrl = server.url("/").toString()
        val result = client.fetchBlocklist(
            baseUrl = baseUrl,
            jwtToken = "test_jwt_token",
            ifNoneMatch = "v1-oldversion"
        )

        val recordedRequest = server.takeRequest()
        assertEquals("GET", recordedRequest.method)
        assertEquals("/enforcement/blocklist", recordedRequest.path)
        assertEquals("Bearer test_jwt_token", recordedRequest.getHeader("Authorization"))
        assertEquals("v1-oldversion", recordedRequest.getHeader("If-None-Match"))

        assertTrue(result is SyncResult.Success)
        val success = result as SyncResult.Success
        assertEquals("user_api_test", success.response.userId)
        assertEquals("v1-0123456789abcdef", success.response.version)
        assertEquals(1, success.response.targets.size)
        assertEquals("com.instagram.android", success.response.targets[0].apps[0])
    }

    @Test
    fun testFetchBlocklist_304NotModified() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(304)
        )

        val baseUrl = server.url("/").toString()
        val result = client.fetchBlocklist(
            baseUrl = baseUrl,
            jwtToken = "test_jwt_token",
            ifNoneMatch = "v1-currentversion"
        )

        val recordedRequest = server.takeRequest()
        assertEquals("v1-currentversion", recordedRequest.getHeader("If-None-Match"))

        assertTrue(result is SyncResult.NotModified)
    }

    @Test
    fun testFetchBlocklist_401Unauthorized() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(401)
                .setBody("""{"error":"unauthorized"}""")
        )

        val baseUrl = server.url("/").toString()
        val result = client.fetchBlocklist(
            baseUrl = baseUrl,
            jwtToken = "expired_token"
        )

        assertTrue(result is SyncResult.Error)
        val error = result as SyncResult.Error
        assertEquals(401, error.statusCode)
    }

    @Test
    fun testFetchBlocklist_500ServerError() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(500)
                .setBody("""{"error":"internal server error"}""")
        )

        val baseUrl = server.url("/").toString()
        val result = client.fetchBlocklist(
            baseUrl = baseUrl,
            jwtToken = "test_jwt_token"
        )

        assertTrue(result is SyncResult.Error)
        val error = result as SyncResult.Error
        assertEquals(500, error.statusCode)
    }
}
