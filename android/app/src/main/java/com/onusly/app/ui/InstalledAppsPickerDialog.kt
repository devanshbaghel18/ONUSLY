package com.onusly.app.ui

import android.app.Dialog
import android.content.Context
import android.text.Editable
import android.text.TextWatcher
import android.view.LayoutInflater
import android.view.View
import android.widget.EditText
import android.widget.TextView
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.button.MaterialButton
import com.google.android.material.chip.Chip
import com.google.android.material.chip.ChipGroup
import com.onusly.app.R
import com.onusly.app.data.model.AppCategory
import com.onusly.app.data.model.InstalledAppInfo
import com.onusly.app.engine.InstalledAppsManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class InstalledAppsPickerDialog(
    private val context: Context,
    private val initialSelected: Set<String>,
    private val onSelected: (Set<String>) -> Unit
) {

    private val dialog = Dialog(context, android.R.style.Theme_Material_Dialog)
    private var allApps: List<InstalledAppInfo> = emptyList()
    private var scanJob: Job? = null
    private var selectedCategory: AppCategory = AppCategory.ALL

    fun show() {
        val view = LayoutInflater.from(context).inflate(R.layout.dialog_app_picker, null)
        dialog.setContentView(view)
        dialog.window?.setLayout(
            (context.resources.displayMetrics.widthPixels * 0.95).toInt(),
            (context.resources.displayMetrics.heightPixels * 0.80).toInt()
        )

        val etSearchApps: EditText = view.findViewById(R.id.etSearchApps)
        val chipGroupCategories: ChipGroup = view.findViewById(R.id.chipGroupCategories)
        val chipCatAll: Chip = view.findViewById(R.id.chipCatAll)
        val chipCatSocial: Chip = view.findViewById(R.id.chipCatSocial)
        val chipCatEntertainment: Chip = view.findViewById(R.id.chipCatEntertainment)
        val chipCatMessaging: Chip = view.findViewById(R.id.chipCatMessaging)
        val chipCatGaming: Chip = view.findViewById(R.id.chipCatGaming)

        val etManualPackage: EditText = view.findViewById(R.id.etManualPackage)
        val btnAddManualPackage: MaterialButton = view.findViewById(R.id.btnAddManualPackage)
        val tvManualPackageError: TextView = view.findViewById(R.id.tvManualPackageError)
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

        fun updateAppList() {
            val query = etSearchApps.text?.toString()
            val filtered = InstalledAppsManager.filterApps(allApps, query, selectedCategory)
            adapter.setApps(filtered)
        }

        chipCatAll.setOnClickListener { selectedCategory = AppCategory.ALL; updateAppList() }
        chipCatSocial.setOnClickListener { selectedCategory = AppCategory.SOCIAL; updateAppList() }
        chipCatEntertainment.setOnClickListener { selectedCategory = AppCategory.ENTERTAINMENT; updateAppList() }
        chipCatMessaging.setOnClickListener { selectedCategory = AppCategory.MESSAGING; updateAppList() }
        chipCatGaming.setOnClickListener { selectedCategory = AppCategory.GAMING; updateAppList() }

        // Load apps asynchronously on IO dispatcher
        scanJob = CoroutineScope(Dispatchers.Main).launch {
            val installed = withContext(Dispatchers.IO) {
                InstalledAppsManager.getInstalledApps(context)
            }
            // Supplement with known catalog distractors so they are easily selectable
            val catalog = InstalledAppsManager.getCatalogApps()
            val combined = (installed + catalog.filter { cat -> installed.none { it.packageName.equals(cat.packageName, ignoreCase = true) } })
                .sortedBy { it.label.lowercase() }
            allApps = combined
            updateAppList()
        }

        dialog.setOnDismissListener {
            scanJob?.cancel()
            scanJob = null
        }

        dialog.setOnCancelListener {
            scanJob?.cancel()
            scanJob = null
        }

        etSearchApps.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                updateAppList()
            }
            override fun afterTextChanged(s: Editable?) {}
        })

        btnAddManualPackage.setOnClickListener {
            val input = etManualPackage.text?.toString()?.trim() ?: ""
            when (val valResult = InstalledAppsManager.validatePackageForTargeting(input)) {
                is InstalledAppsManager.ValidationResult.Invalid -> {
                    tvManualPackageError.text = valResult.reason
                    tvManualPackageError.visibility = View.VISIBLE
                }
                is InstalledAppsManager.ValidationResult.Valid -> {
                    tvManualPackageError.visibility = View.GONE
                    etManualPackage.text?.clear()
                    adapter.selectedPackages.add(input)
                    tvSelectedCount.text = "${adapter.selectedPackages.size} selected"
                    if (allApps.none { it.packageName.equals(input, ignoreCase = true) }) {
                        val newApp = InstalledAppInfo(
                            packageName = input,
                            label = input,
                            icon = null,
                            category = InstalledAppsManager.resolveCategory(input, input)
                        )
                        allApps = listOf(newApp) + allApps
                        updateAppList()
                    } else {
                        adapter.notifyDataSetChanged()
                    }
                }
            }
        }

        btnCancelPicker.setOnClickListener {
            try { if (dialog.isShowing) dialog.dismiss() } catch (_: Exception) {}
        }

        btnConfirmPicker.setOnClickListener {
            onSelected(adapter.selectedPackages)
            try { if (dialog.isShowing) dialog.dismiss() } catch (_: Exception) {}
        }

        dialog.show()
    }
}
