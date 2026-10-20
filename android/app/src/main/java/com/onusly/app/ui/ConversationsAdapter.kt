package com.onusly.app.ui

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import com.onusly.app.R
import com.onusly.app.data.model.ConversationItem

class ConversationsAdapter(
    private val onConversationClicked: (ConversationItem) -> Unit
) : RecyclerView.Adapter<ConversationsAdapter.ConversationViewHolder>() {

    private val items = mutableListOf<ConversationItem>()

    fun setItems(newItems: List<ConversationItem>) {
        items.clear()
        items.addAll(newItems)
        notifyDataSetChanged()
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ConversationViewHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_conversation, parent, false)
        return ConversationViewHolder(view)
    }

    override fun onBindViewHolder(holder: ConversationViewHolder, position: Int) {
        holder.bind(items[position], onConversationClicked)
    }

    override fun getItemCount(): Int = items.size

    class ConversationViewHolder(itemView: View) : RecyclerView.ViewHolder(itemView) {
        private val tvName: TextView = itemView.findViewById(R.id.tvConvoName)
        private val tvBadge: TextView = itemView.findViewById(R.id.tvConvoBadge)
        private val tvSubtitle: TextView = itemView.findViewById(R.id.tvConvoSubtitle)
        private val tvTime: TextView = itemView.findViewById(R.id.tvConvoTime)
        private val ivIcon: ImageView = itemView.findViewById(R.id.ivConvoIcon)
        private val tvInitial: TextView = itemView.findViewById(R.id.tvConvoInitial)

        fun bind(item: ConversationItem, onClick: (ConversationItem) -> Unit) {
            tvName.text = item.name
            tvSubtitle.text = item.subtitle
            tvTime.text = item.time

            when (item) {
                is ConversationItem.FriendConversation -> {
                    tvBadge.visibility = View.GONE
                    val firstChar = item.name.removePrefix("@").firstOrNull()?.uppercase() ?: "F"
                    tvInitial.text = firstChar
                    tvInitial.visibility = View.VISIBLE
                    ivIcon.visibility = View.GONE
                }
                is ConversationItem.CommunityConversation -> {
                    tvBadge.visibility = View.VISIBLE
                    tvBadge.text = item.community.category.uppercase()
                    tvInitial.visibility = View.GONE
                    ivIcon.visibility = View.VISIBLE
                    ivIcon.setImageResource(R.drawable.ic_group)
                }
            }

            itemView.setOnClickListener {
                onClick(item)
            }
        }
    }
}
