package com.onusly.app.ui

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.CheckBox
import android.widget.ImageView
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import com.onusly.app.R
import com.onusly.app.data.model.InstalledAppInfo

class InstalledAppsAdapter(
    initialSelected: Set<String>,
    private val onSelectionChanged: (Int) -> Unit
) : RecyclerView.Adapter<InstalledAppsAdapter.AppViewHolder>() {

    private var appsList: List<InstalledAppInfo> = emptyList()
    val selectedPackages: MutableSet<String> = initialSelected.toMutableSet()

    fun setApps(apps: List<InstalledAppInfo>) {
        this.appsList = apps
        notifyDataSetChanged()
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): AppViewHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_app_picker, parent, false)
        return AppViewHolder(view)
    }

    override fun onBindViewHolder(holder: AppViewHolder, position: Int) {
        holder.bind(appsList[position])
    }

    override fun getItemCount(): Int = appsList.size

    inner class AppViewHolder(itemView: View) : RecyclerView.ViewHolder(itemView) {
        private val ivAppIcon: ImageView = itemView.findViewById(R.id.ivAppIcon)
        private val tvAppLabel: TextView = itemView.findViewById(R.id.tvAppLabel)
        private val tvAppPackage: TextView = itemView.findViewById(R.id.tvAppPackage)
        private val cbAppSelected: CheckBox = itemView.findViewById(R.id.cbAppSelected)

        fun bind(app: InstalledAppInfo) {
            tvAppLabel.text = app.label
            tvAppPackage.text = app.packageName

            if (app.icon != null) {
                ivAppIcon.setImageDrawable(app.icon)
            } else {
                ivAppIcon.setImageResource(R.drawable.ic_apps)
            }

            val isSelected = selectedPackages.contains(app.packageName)
            cbAppSelected.isChecked = isSelected

            itemView.setOnClickListener {
                if (selectedPackages.contains(app.packageName)) {
                    selectedPackages.remove(app.packageName)
                } else {
                    selectedPackages.add(app.packageName)
                }
                cbAppSelected.isChecked = selectedPackages.contains(app.packageName)
                onSelectionChanged(selectedPackages.size)
            }
        }
    }
}
