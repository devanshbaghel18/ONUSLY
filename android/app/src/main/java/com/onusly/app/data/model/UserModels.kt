package com.onusly.app.data.model

data class UserProfile(
    val id: String,
    val email: String,
    val handle: String = "",
    val name: String = "",
    val picture: String = "",
    val createdAt: String = ""
)

data class PublicProfile(
    val id: String,
    val name: String = "",
    val handle: String = "",
    val picture: String = ""
)
