package com.onusly.app.ui

import android.graphics.Color
import android.view.Gravity
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.button.MaterialButton
import com.google.android.material.card.MaterialCardView
import com.onusly.app.R
import com.onusly.app.data.model.ChatMessage
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class ChatAdapter(
    private val currentUserId: String,
    private val currentUserEmail: String,
    private val currentUserHandle: String,
    private val onApproveClicked: (ChatMessage) -> Unit,
    private val onRejectClicked: (ChatMessage) -> Unit
) : RecyclerView.Adapter<ChatAdapter.ChatViewHolder>() {

    private val messages = mutableListOf<ChatMessage>()

    fun setMessages(newMessages: List<ChatMessage>): Boolean {
        if (messages.size == newMessages.size && messages == newMessages) {
            return false
        }
        messages.clear()
        messages.addAll(newMessages)
        notifyDataSetChanged()
        return true
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ChatViewHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_chat_message, parent, false)
        return ChatViewHolder(view)
    }

    override fun onBindViewHolder(holder: ChatViewHolder, position: Int) {
        holder.bind(messages[position])
    }

    override fun getItemCount(): Int = messages.size

    inner class ChatViewHolder(itemView: View) : RecyclerView.ViewHolder(itemView) {
        private val layoutRoot: LinearLayout = itemView.findViewById(R.id.layoutMessageRoot)
        private val tvSender: TextView = itemView.findViewById(R.id.tvChatSender)
        private val cardBubble: MaterialCardView = itemView.findViewById(R.id.cardMessageBubble)
        private val layoutVerificationHeader: LinearLayout = itemView.findViewById(R.id.layoutVerificationHeader)
        private val tvGoalTitle: TextView = itemView.findViewById(R.id.tvChatGoalTitle)
        private val tvMessageText: TextView = itemView.findViewById(R.id.tvChatMessageText)
        private val layoutActions: LinearLayout = itemView.findViewById(R.id.layoutVerificationActions)
        private val btnApprove: MaterialButton = itemView.findViewById(R.id.btnChatApprove)
        private val btnReject: MaterialButton = itemView.findViewById(R.id.btnChatReject)
        private val tvApprovedStatus: TextView = itemView.findViewById(R.id.tvChatApprovedStatus)
        private val tvTime: TextView = itemView.findViewById(R.id.tvChatMessageTime)

        fun bind(msg: ChatMessage) {
            val isMe = isMessageFromMe(msg)

            // Bubble alignment
            layoutRoot.gravity = if (isMe) Gravity.END else Gravity.START

            if (isMe) {
                tvSender.visibility = View.GONE
                cardBubble.setCardBackgroundColor(Color.parseColor("#FFFFFF"))
                cardBubble.strokeColor = Color.parseColor("#E0E0E0")
                tvMessageText.setTextColor(Color.parseColor("#000000"))
                tvTime.setTextColor(Color.parseColor("#666666"))
            } else {
                val senderName = when {
                    msg.senderHandle.isNotBlank() -> "@${msg.senderHandle.removePrefix("@")}"
                    msg.senderEmail.isNotBlank() -> msg.senderEmail
                    else -> "Partner"
                }
                tvSender.text = senderName
                tvSender.visibility = View.VISIBLE
                cardBubble.setCardBackgroundColor(Color.parseColor("#1E1E24"))
                cardBubble.strokeColor = Color.parseColor("#333338")
                tvMessageText.setTextColor(Color.parseColor("#FFFFFF"))
                tvTime.setTextColor(Color.parseColor("#888888"))
            }

            // Message text
            tvMessageText.text = msg.text

            // Timestamp formatting
            tvTime.text = formatTime(msg.time)

            // Verification Card
            val isVerification = msg.isProof || msg.text.contains("verify my goal", ignoreCase = true)
            if (isVerification) {
                layoutVerificationHeader.visibility = View.VISIBLE
                val title = if (msg.goalTitle.isNotBlank()) msg.goalTitle else "Goal Verification"
                tvGoalTitle.text = "🎯 $title"

                if (isMe) {
                    tvGoalTitle.setTextColor(Color.parseColor("#000000"))
                } else {
                    tvGoalTitle.setTextColor(Color.parseColor("#FFFFFF"))
                }

                if (msg.approved) {
                    tvApprovedStatus.visibility = View.VISIBLE
                    layoutActions.visibility = View.GONE
                } else {
                    tvApprovedStatus.visibility = View.GONE
                    // If not me and goal is pending: show Approve & Reject buttons!
                    if (!isMe) {
                        layoutActions.visibility = View.VISIBLE
                        btnApprove.setOnClickListener { onApproveClicked(msg) }
                        btnReject.setOnClickListener { onRejectClicked(msg) }
                    } else {
                        layoutActions.visibility = View.GONE
                    }
                }
            } else {
                layoutVerificationHeader.visibility = View.GONE
                layoutActions.visibility = View.GONE
                tvApprovedStatus.visibility = View.GONE
            }
        }

        private fun isMessageFromMe(msg: ChatMessage): Boolean {
            val cleanCallerID = currentUserId.trim()
            val cleanSenderID = msg.senderID().trim()
            if (cleanCallerID.isNotBlank() && cleanSenderID.isNotBlank() && cleanCallerID.equals(cleanSenderID, ignoreCase = true)) {
                return true
            }

            val cleanCallerEmail = currentUserEmail.trim().lowercase()
            val cleanSenderEmail = msg.senderEmail.trim().lowercase()
            if (cleanCallerEmail.isNotBlank() && cleanSenderEmail.isNotBlank() && cleanCallerEmail == cleanSenderEmail) {
                return true
            }

            val cleanCallerHandle = currentUserHandle.trim().removePrefix("@").lowercase()
            val cleanSenderHandle = msg.senderHandle.trim().removePrefix("@").lowercase()
            if (cleanCallerHandle.isNotBlank() && cleanSenderHandle.isNotBlank() && cleanCallerHandle == cleanSenderHandle) {
                return true
            }

            return false
        }

        private fun formatTime(timeStr: String): String {
            if (timeStr.isBlank()) return ""
            return try {
                val iso = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss", Locale.US)
                val out = SimpleDateFormat("h:mm a", Locale.getDefault())
                val d = iso.parse(timeStr.substringBefore("."))
                if (d != null) out.format(d) else timeStr
            } catch (_: Exception) {
                timeStr
            }
        }
    }
}

private fun ChatMessage.senderID(): String = this.senderId
