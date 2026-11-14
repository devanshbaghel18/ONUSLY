package com.onusly.app.ui

import android.app.Dialog
import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.EditText
import android.widget.ImageView
import android.widget.ProgressBar
import android.widget.RadioButton
import android.widget.RadioGroup
import android.widget.TextView
import com.google.android.material.button.MaterialButton
import com.onusly.app.R
import com.onusly.app.data.GoalsRepository
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.model.CreateProofRequest
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.Proof
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.net.URI

class SubmitProofDialog(
    private val context: Context,
    private val scope: CoroutineScope,
    private val goalsRepository: GoalsRepository,
    private val goal: Goal,
    private val onProofSubmitted: (Proof, Boolean, String?) -> Unit
) {

    private val dialog: Dialog = Dialog(context, android.R.style.Theme_Material_Dialog)

    private lateinit var tvSubmitProofModalSubtitle: TextView
    private lateinit var tvSubmitProofError: TextView
    private lateinit var rgProofType: RadioGroup
    private lateinit var rbProofText: RadioButton
    private lateinit var rbProofLink: RadioButton
    private lateinit var rbProofPhoto: RadioButton
    private lateinit var layoutProofLink: View
    private lateinit var etProofLink: EditText
    private lateinit var layoutProofPhoto: View
    private lateinit var etProofPhotoUrl: EditText
    private lateinit var btnPickPhotoGallery: TextView
    private lateinit var layoutPhotoUploadProgress: View
    private lateinit var pbPhotoUpload: ProgressBar
    private lateinit var tvPhotoUploadStatus: TextView
    private lateinit var ivProofPhotoPreview: ImageView
    private lateinit var tvExplanationLabel: TextView
    private lateinit var etProofExplanation: EditText
    private lateinit var pbSubmitProof: ProgressBar
    private lateinit var btnCancelProof: MaterialButton
    private lateinit var btnConfirmSubmitProof: MaterialButton

    private var currentProofType: String = "text"

    fun show() {
        val view = LayoutInflater.from(context).inflate(R.layout.dialog_submit_proof, null)
        dialog.setContentView(view)
        dialog.window?.setLayout(
            (context.resources.displayMetrics.widthPixels * 0.95).toInt(),
            ViewGroup.LayoutParams.WRAP_CONTENT
        )

        bindViews(view)
        setupInitialData()
        setupListeners()

        dialog.show()
    }

    private fun bindViews(view: View) {
        tvSubmitProofModalSubtitle = view.findViewById(R.id.tvSubmitProofModalSubtitle)
        tvSubmitProofError = view.findViewById(R.id.tvSubmitProofError)
        rgProofType = view.findViewById(R.id.rgProofType)
        rbProofText = view.findViewById(R.id.rbProofText)
        rbProofLink = view.findViewById(R.id.rbProofLink)
        rbProofPhoto = view.findViewById(R.id.rbProofPhoto)
        layoutProofLink = view.findViewById(R.id.layoutProofLink)
        etProofLink = view.findViewById(R.id.etProofLink)
        layoutProofPhoto = view.findViewById(R.id.layoutProofPhoto)
        etProofPhotoUrl = view.findViewById(R.id.etProofPhotoUrl)
        btnPickPhotoGallery = view.findViewById(R.id.btnPickPhotoGallery)
        layoutPhotoUploadProgress = view.findViewById(R.id.layoutPhotoUploadProgress)
        pbPhotoUpload = view.findViewById(R.id.pbPhotoUpload)
        tvPhotoUploadStatus = view.findViewById(R.id.tvPhotoUploadStatus)
        ivProofPhotoPreview = view.findViewById(R.id.ivProofPhotoPreview)
        tvExplanationLabel = view.findViewById(R.id.tvExplanationLabel)
        etProofExplanation = view.findViewById(R.id.etProofExplanation)
        pbSubmitProof = view.findViewById(R.id.pbSubmitProof)
        btnCancelProof = view.findViewById(R.id.btnCancelProof)
        btnConfirmSubmitProof = view.findViewById(R.id.btnConfirmSubmitProof)
    }

    private fun setupInitialData() {
        tvSubmitProofModalSubtitle.text = "Goal: ${goal.title}"
        updateFieldVisibility("text")
    }

    private fun setupListeners() {
        rgProofType.setOnCheckedChangeListener { _, checkedId ->
            when (checkedId) {
                R.id.rbProofText -> updateFieldVisibility("text")
                R.id.rbProofLink -> updateFieldVisibility("link")
                R.id.rbProofPhoto -> updateFieldVisibility("photo")
            }
        }

        btnPickPhotoGallery.setOnClickListener {
            (context as? MainActivity)?.pickProofImage { uri ->
                handleSelectedImage(uri)
            }
        }

        btnCancelProof.setOnClickListener {
            dialog.dismiss()
        }

        btnConfirmSubmitProof.setOnClickListener {
            validateAndSubmit()
        }
    }

    private fun getFileNameFromUri(context: Context, uri: Uri): String? {
        if (uri.scheme == "content") {
            try {
                context.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
                    val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                    if (nameIndex != -1 && cursor.moveToFirst()) {
                        return cursor.getString(nameIndex)
                    }
                }
            } catch (_: Exception) {}
        }
        return uri.path?.let { p ->
            val cut = p.lastIndexOf('/')
            if (cut != -1) p.substring(cut + 1) else p
        }
    }

    private fun handleSelectedImage(uri: Uri) {
        hideError()
        layoutPhotoUploadProgress.visibility = View.VISIBLE
        pbPhotoUpload.visibility = View.VISIBLE
        tvPhotoUploadStatus.text = "Uploading evidence to server..."
        ivProofPhotoPreview.visibility = View.VISIBLE
        ivProofPhotoPreview.setImageURI(uri)

        scope.launch(Dispatchers.IO) {
            val fileName = getFileNameFromUri(context, uri) ?: "evidence_${System.currentTimeMillis()}.jpg"
            val mimeType = context.contentResolver.getType(uri) ?: "image/jpeg"
            val bytes = try {
                context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            } catch (e: Exception) {
                null
            }

            withContext(Dispatchers.Main) {
                if (bytes == null || bytes.isEmpty()) {
                    layoutPhotoUploadProgress.visibility = View.GONE
                    showError("Could not read selected photo file.")
                    return@withContext
                }

                val uploadResult = withContext(Dispatchers.IO) {
                    goalsRepository.uploadProofFile(fileName, mimeType, bytes)
                }

                uploadResult.onSuccess { uploadedItem ->
                    etProofPhotoUrl.setText(uploadedItem.url)
                    layoutPhotoUploadProgress.visibility = View.VISIBLE
                    pbPhotoUpload.visibility = View.GONE
                    tvPhotoUploadStatus.text = "Uploaded: ${uploadedItem.name} (${uploadedItem.formattedSize})"
                }.onFailure { err ->
                    layoutPhotoUploadProgress.visibility = View.GONE
                    showError("Upload failed: ${err.message}. You may also paste an external URL.")
                }
            }
        }
    }

    private fun updateFieldVisibility(type: String) {
        currentProofType = type
        when (type) {
            "text" -> {
                layoutProofLink.visibility = View.GONE
                layoutProofPhoto.visibility = View.GONE
                tvExplanationLabel.text = "EXPLANATION *"
                etProofExplanation.hint = "Describe the work done, milestones achieved, or reflection..."
            }
            "link" -> {
                layoutProofLink.visibility = View.VISIBLE
                layoutProofPhoto.visibility = View.GONE
                tvExplanationLabel.text = "EXPLANATION (OPTIONAL)"
                etProofExplanation.hint = "Optional context about this link..."
            }
            "photo" -> {
                layoutProofLink.visibility = View.GONE
                layoutProofPhoto.visibility = View.VISIBLE
                tvExplanationLabel.text = "EXPLANATION (OPTIONAL)"
                etProofExplanation.hint = "Optional context about this photo..."
            }
        }
    }

    private fun validateAndSubmit() {
        hideError()

        val explanation = etProofExplanation.text.toString().trim()
        val link = etProofLink.text.toString().trim()
        val photoUrl = etProofPhotoUrl.text.toString().trim()

        when (currentProofType) {
            "text" -> {
                if (explanation.isBlank()) {
                    showError("Please enter an explanation of your completed work.")
                    return
                }
            }
            "link" -> {
                if (link.isBlank()) {
                    showError("Please enter an external link URL.")
                    return
                }
                if (!isValidUrl(link)) {
                    showError("Please enter a valid link URL starting with http:// or https://.")
                    return
                }
            }
            "photo" -> {
                if (photoUrl.isBlank() && explanation.isBlank()) {
                    showError("Please enter a photo URL or provide an explanation.")
                    return
                }
                if (photoUrl.isNotBlank() && !isValidUrl(photoUrl)) {
                    showError("Please enter a valid photo URL starting with http:// or https://.")
                    return
                }
            }
        }

        val request = CreateProofRequest(
            proofType = currentProofType,
            textExplanation = explanation,
            externalLink = if (currentProofType == "link") link else "",
            photoUrl = if (currentProofType == "photo") photoUrl else ""
        )

        setSubmittingState(true)

        scope.launch(Dispatchers.IO) {
            val result = goalsRepository.submitProof(goal.id, request)
            withContext(Dispatchers.Main) {
                setSubmittingState(false)
                result.onSuccess { mutation ->
                    try { if (dialog.isShowing) dialog.dismiss() } catch (_: Exception) {}
                    onProofSubmitted(mutation.value, mutation.syncSucceeded, mutation.syncError)
                }.onFailure { err ->
                    showError(formatErrorMessage(err))
                }
            }
        }
    }

    private fun setSubmittingState(submitting: Boolean) {
        pbSubmitProof.visibility = if (submitting) View.VISIBLE else View.GONE
        btnConfirmSubmitProof.isEnabled = !submitting
        btnCancelProof.isEnabled = !submitting
        rbProofText.isEnabled = !submitting
        rbProofLink.isEnabled = !submitting
        rbProofPhoto.isEnabled = !submitting
        etProofExplanation.isEnabled = !submitting
        etProofLink.isEnabled = !submitting
        etProofPhotoUrl.isEnabled = !submitting
    }

    private fun showError(message: String) {
        tvSubmitProofError.text = message
        tvSubmitProofError.visibility = View.VISIBLE
    }

    private fun hideError() {
        tvSubmitProofError.visibility = View.GONE
    }

    private fun formatErrorMessage(err: Throwable): String {
        return when (err) {
            is OnuslyApiException -> {
                when {
                    err.statusCode == 409 -> "Proof has already been submitted for this goal."
                    err.statusCode == 404 -> "Goal not found. It may have been deleted."
                    err.statusCode == 401 -> "Session expired. Please sign in again."
                    else -> err.message
                }
            }
            else -> err.message ?: "Failed to submit proof. Please check your network connection."
        }
    }

    companion object {
        fun isValidUrl(url: String): Boolean {
            val trimmed = url.trim()
            if (trimmed.startsWith("/uploads/", ignoreCase = true)) {
                return true
            }
            if (trimmed.startsWith("content://", ignoreCase = true) || trimmed.startsWith("file://", ignoreCase = true)) {
                return true
            }
            if (!trimmed.startsWith("http://", ignoreCase = true) && !trimmed.startsWith("https://", ignoreCase = true)) {
                return false
            }
            return try {
                val uri = URI(trimmed)
                !uri.host.isNullOrBlank()
            } catch (_: Exception) {
                false
            }
        }
    }
}
