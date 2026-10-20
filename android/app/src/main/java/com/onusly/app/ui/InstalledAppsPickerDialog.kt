package com.onusly.app.ui

import android.app.Dialog
import android.content.Context
import android.text.Editable
import android.text.TextWatcher
import android.view.LayoutInflater
import android.view.ViewGroup
import android.widget.EditText
import android.widget.TextView
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.button.MaterialButton
import com.onusly.app.R
import com.onusly.app.data.model.InstalledAppInfo
import com.onusly.app.engine.InstalledAppsManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class InstalledAppsPickerDialog(
    private val context: Context,
    private val initialSelected: Set<String>,
    private val onSelected: (Set<String>) -> Unit
) {

    private val dialog = Dialog(context, android.R.style.Theme_Material_Dialog)
    private var allApps: List<InstalledAppInfo> = emptyList()

    fun show() {
        val view = LayoutInflater.from(context).inflate(R.layout.dialog_app_picker, null)
        dialog.setContentView(view)
        dialog.window?.setLayout(
            (context.resources.displayMetrics.widthPixels * 0.95).toInt(),
            (context.resources.displayMetrics.heightPixels * 0.75).toInt()
        )

        val etSearchApps: EditText = view.findViewById(R.id.etSearchApps)
        val tvSelectedCount: TextView = view.findViewById(R.id.tvSelectedCount)
        val rvInstalledApps: RecyclerView = view.findViewById(R.id.rvInstalledApps)
        val btnCancelPicker: MaterialButton = view.findViewById(R.id.btnCancelPicker)
        val btnConfirmPicker: MaterialButton = view.findViewById(R.id.btnConfirmPicker)

        val adapter = InstalledAppsAdapter(initialSelected) { count ->
            tvSelectedCount.text = "$count selected"
        }

        rvInstalledApps.layoutManager = LinearLayoutManager(context)
        rvInstalledApps.adapter = adapter
        tvSelectedCount.text = "${adapter.selectedPackages.size} selected"

        // Load apps asynchronously on IO dispatcher
        CoroutineScope(Dispatchers.Main).launch {
            allApps = withContext(Dispatchers.IO) {
                InstalledAppsManager.getInstalledApps(context)
            }
            adapter.setApps(allApps)
        }

        etSearchApps.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                val filtered = InstalledAppsManager.filterApps(allApps, s?.toString())
                adapter.setApps(filtered)
            }
            override fun afterTextChanged(s: Editable?) {}
        })

        btnCancelPicker.setOnClickListener {
            dialog.dismiss()
        }

        btnConfirmPicker.setOnClickListener {
            onSelected(adapter.selectedPackages)
            dialog.dismiss()
        }

        dialog.show()
    }
}
