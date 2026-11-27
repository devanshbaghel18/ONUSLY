package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
	"github.com/google/uuid"
)

const maxSingleUploadBytes = 25 << 20 // 25MB per file ceiling

// UploadUserResolver resolves a chat recipient handle/email/id for chat upload ACLs.
type UploadUserResolver interface {
	ResolveUser(ctx context.Context, identifier string) (userID, email, handle string, err error)
}

type UploadedFileItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	FormattedSize string `json:"formattedSize"`
	Type          string `json:"type"`
	URL           string `json:"url"`
	DataURL       string `json:"dataUrl"`
	IsImage       bool   `json:"isImage"`
	IsPdf         bool   `json:"isPdf"`
}

type uploadExtSpec struct {
	contentType string
	isImage     bool
	isInline    bool
	kind        string // "image", "pdf", "text", "zip_ooxml", "ole2", "isobmff", "bmp", "mp3", "riff"
}

var allowedUploadSpecs = map[string]uploadExtSpec{
	".png":  {contentType: "image/png", isImage: true, isInline: true, kind: "image"},
	".jpg":  {contentType: "image/jpeg", isImage: true, isInline: true, kind: "image"},
	".jpeg": {contentType: "image/jpeg", isImage: true, isInline: true, kind: "image"},
	".webp": {contentType: "image/webp", isImage: true, isInline: true, kind: "image"},
	".gif":  {contentType: "image/gif", isImage: true, isInline: true, kind: "image"},
	".bmp":  {contentType: "image/bmp", isImage: true, isInline: true, kind: "bmp"},
	".avif": {contentType: "image/avif", isImage: true, isInline: true, kind: "isobmff"},
	".heic": {contentType: "image/heic", isImage: true, isInline: true, kind: "isobmff"},
	".heif": {contentType: "image/heif", isImage: true, isInline: true, kind: "isobmff"},
	".pdf":  {contentType: "application/pdf", isImage: false, isInline: true, kind: "pdf"},
	".txt":  {contentType: "text/plain; charset=utf-8", isImage: false, isInline: false, kind: "text"},
	".md":   {contentType: "text/markdown; charset=utf-8", isImage: false, isInline: false, kind: "text"},
	".csv":  {contentType: "text/csv; charset=utf-8", isImage: false, isInline: false, kind: "text"},
	".json": {contentType: "application/json; charset=utf-8", isImage: false, isInline: false, kind: "text"},
	".rtf":  {contentType: "application/rtf", isImage: false, isInline: false, kind: "text"},
	".doc":  {contentType: "application/msword", isImage: false, isInline: false, kind: "ole2"},
	".xls":  {contentType: "application/vnd.ms-excel", isImage: false, isInline: false, kind: "ole2"},
	".ppt":  {contentType: "application/vnd.ms-powerpoint", isImage: false, isInline: false, kind: "ole2"},
	".docx": {contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", isImage: false, isInline: false, kind: "zip_ooxml"},
	".xlsx": {contentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", isImage: false, isInline: false, kind: "zip_ooxml"},
	".pptx": {contentType: "application/vnd.openxmlformats-officedocument.presentationml.presentation", isImage: false, isInline: false, kind: "zip_ooxml"},
	".odt":  {contentType: "application/vnd.oasis.opendocument.text", isImage: false, isInline: false, kind: "zip_ooxml"},
	".zip":  {contentType: "application/zip", isImage: false, isInline: false, kind: "zip_ooxml"},
	".mp4":  {contentType: "video/mp4", isImage: false, isInline: false, kind: "isobmff"},
	".webm": {contentType: "video/webm", isImage: false, isInline: false, kind: "webm"},
	".mp3":  {contentType: "audio/mpeg", isImage: false, isInline: false, kind: "mp3"},
	".wav":  {contentType: "audio/wav", isImage: false, isInline: false, kind: "riff"},
}

func isExecutableOrHTML(sniff []byte, detectedMIME string) bool {
	if strings.Contains(detectedMIME, "text/html") || strings.Contains(detectedMIME, "application/x-msdownload") {
		return true
	}
	if len(sniff) >= 2 && sniff[0] == 'M' && sniff[1] == 'Z' {
		return true
	}
	if len(sniff) >= 4 && sniff[0] == 0x7f && sniff[1] == 'E' && sniff[2] == 'L' && sniff[3] == 'F' {
		return true
	}
	if len(sniff) >= 2 && sniff[0] == '#' && sniff[1] == '!' {
		return true
	}
	trimmed := bytes.TrimSpace(sniff)
	lower := strings.ToLower(string(trimmed))
	return strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") || strings.HasPrefix(lower, "<script")
}

func validateUploadSniff(ext string, spec uploadExtSpec, sniff []byte) bool {
	detected := strings.ToLower(http.DetectContentType(sniff))
	if isExecutableOrHTML(sniff, detected) {
		return false
	}

	switch spec.kind {
	case "image":
		switch ext {
		case ".png":
			return detected == "image/png"
		case ".jpg", ".jpeg":
			return detected == "image/jpeg"
		case ".webp":
			return detected == "image/webp"
		case ".gif":
			return detected == "image/gif"
		}
		return false
	case "pdf":
		return detected == "application/pdf" || bytes.HasPrefix(bytes.TrimSpace(sniff), []byte("%PDF-"))
	case "bmp":
		return detected == "image/bmp" || (len(sniff) >= 2 && sniff[0] == 'B' && sniff[1] == 'M')
	case "isobmff":
		return len(sniff) >= 12 && bytes.Equal(sniff[4:8], []byte("ftyp"))
	case "zip_ooxml":
		return len(sniff) >= 4 && sniff[0] == 'P' && sniff[1] == 'K' && (sniff[2] == 0x03 || sniff[2] == 0x05)
	case "ole2":
		return len(sniff) >= 8 && bytes.Equal(sniff[:8], []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	case "webm":
		return len(sniff) >= 4 && bytes.Equal(sniff[:4], []byte{0x1A, 0x45, 0xDF, 0xA3})
	case "mp3":
		return detected == "audio/mpeg" || (len(sniff) >= 3 && string(sniff[:3]) == "ID3") || (len(sniff) >= 2 && sniff[0] == 0xFF && (sniff[1]&0xE0) == 0xE0)
	case "riff":
		return len(sniff) >= 12 && string(sniff[:4]) == "RIFF" && string(sniff[8:12]) == "WAVE"
	case "text":
		return strings.HasPrefix(detected, "text/plain") || detected == "application/octet-stream"
	default:
		return false
	}
}

func newUploadCreateHandler(uploadsDir string) http.HandlerFunc {
	return newUploadCreateHandlerWithResolver(uploadsDir, nil)
}

func newUploadCreateHandlerWithResolver(uploadsDir string, userResolver UploadUserResolver) http.HandlerFunc {
	_ = os.MkdirAll(filepath.Join(uploadsDir, "proofs"), 0700)
	_ = os.MkdirAll(filepath.Join(uploadsDir, "chat"), 0700)

	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserID(r.Context())
		if !ok || userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Extend write deadline beyond global 30s timeout for multi-megabyte proof/chat uploads
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute))

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}

		scope := strings.ToLower(strings.TrimSpace(r.FormValue("scope")))
		if scope == "" {
			scope = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
		}
		if scope != "chat" {
			scope = "proofs"
		}

		goalID := strings.TrimSpace(r.FormValue("goalId"))
		if goalID == "" {
			goalID = strings.TrimSpace(r.URL.Query().Get("goalId"))
		}
		recipientHint := strings.TrimSpace(r.FormValue("recipient"))
		communityID := strings.TrimSpace(r.FormValue("communityId"))

		var recipientID string
		if scope == "chat" {
			if communityID == "" && (strings.HasPrefix(recipientHint, "comm-") || strings.HasPrefix(recipientHint, "comm_")) {
				communityID = recipientHint
			} else if recipientHint != "" && userResolver != nil {
				if rID, _, _, err := userResolver.ResolveUser(r.Context(), recipientHint); err == nil && rID != "" {
					recipientID = rID
				}
			}
			if communityID == "" && recipientID == "" && recipientHint == "" {
				http.Error(w, "chat upload requires recipient or communityId", http.StatusBadRequest)
				return
			}
		}

		files := r.MultipartForm.File["files"]
		if len(files) == 0 {
			files = r.MultipartForm.File["file"]
		}
		if len(files) == 0 {
			http.Error(w, "no files provided", http.StatusBadRequest)
			return
		}

		var uploadedItems []UploadedFileItem

		for _, fh := range files {
			if fh.Size > maxSingleUploadBytes {
				http.Error(w, fmt.Sprintf("file %q exceeds 25MB per-file limit", fh.Filename), http.StatusRequestEntityTooLarge)
				return
			}

			ext := strings.ToLower(filepath.Ext(fh.Filename))
			spec, allowed := allowedUploadSpecs[ext]
			if !allowed {
				http.Error(w, fmt.Sprintf("unsupported file type %q", ext), http.StatusBadRequest)
				return
			}

			src, err := fh.Open()
			if err != nil {
				http.Error(w, "failed to open uploaded file: "+err.Error(), http.StatusBadRequest)
				return
			}

			sniffBuf := make([]byte, 512)
			n, readErr := io.ReadFull(src, sniffBuf)
			if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
				src.Close()
				http.Error(w, "failed to read uploaded file: "+readErr.Error(), http.StatusBadRequest)
				return
			}
			if n == 0 {
				src.Close()
				http.Error(w, fmt.Sprintf("uploaded file %q is empty", fh.Filename), http.StatusBadRequest)
				return
			}

			if !validateUploadSniff(ext, spec, sniffBuf[:n]) {
				src.Close()
				http.Error(w, fmt.Sprintf("file content for %q does not match allowed %s format", fh.Filename, ext), http.StatusBadRequest)
				return
			}

			baseName := strings.TrimSuffix(filepath.Base(fh.Filename), ext)
			safeBase := strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
					return r
				}
				return '_'
			}, baseName)
			if safeBase == "" {
				safeBase = "evidence"
			}

			uniqueFileName := fmt.Sprintf("%d-%s-%s%s", time.Now().Unix(), uuid.New().String()[:8], safeBase, ext)
			destDir := filepath.Join(uploadsDir, scope)
			_ = os.MkdirAll(destDir, 0700)
			destPath := filepath.Join(destDir, uniqueFileName)

			dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				src.Close()
				http.Error(w, "failed to store uploaded file", http.StatusInternalServerError)
				return
			}

			if _, err := dst.Write(sniffBuf[:n]); err != nil {
				src.Close()
				dst.Close()
				_ = os.Remove(destPath)
				http.Error(w, "failed to write uploaded file", http.StatusInternalServerError)
				return
			}

			copiedRest, err := io.Copy(dst, io.LimitReader(src, maxSingleUploadBytes-int64(n)+1))
			src.Close()
			dst.Close()
			if err != nil {
				_ = os.Remove(destPath)
				http.Error(w, "failed to save uploaded file", http.StatusInternalServerError)
				return
			}
			copiedSize := int64(n) + copiedRest
			if copiedSize > maxSingleUploadBytes {
				_ = os.Remove(destPath)
				http.Error(w, fmt.Sprintf("file %q exceeds 25MB per-file limit", fh.Filename), http.StatusRequestEntityTooLarge)
				return
			}

			fileURL := fmt.Sprintf("/uploads/%s/%s", scope, uniqueFileName)
			if scope == "chat" {
				if err := proof.RecordChatUploadMetadata(uploadsDir, fileURL, userID, recipientID, communityID); err != nil {
					_ = os.Remove(destPath)
					http.Error(w, "failed to record upload access metadata", http.StatusInternalServerError)
					return
				}
			} else {
				if err := proof.RecordUploadOwner(uploadsDir, fileURL, userID, goalID); err != nil {
					_ = os.Remove(destPath)
					http.Error(w, "failed to record upload access metadata", http.StatusInternalServerError)
					return
				}
			}

			formattedSize := fmt.Sprintf("%d KB", copiedSize/1024)
			if copiedSize < 1024 {
				formattedSize = fmt.Sprintf("%d B", copiedSize)
			} else if copiedSize > 1024*1024 {
				formattedSize = fmt.Sprintf("%.1f MB", float64(copiedSize)/(1024*1024))
			}

			uploadedItems = append(uploadedItems, UploadedFileItem{
				ID:            "file-" + uuid.New().String()[:8],
				Name:          fh.Filename,
				Size:          copiedSize,
				FormattedSize: formattedSize,
				Type:          spec.contentType,
				URL:           fileURL,
				DataURL:       fileURL,
				IsImage:       spec.isImage,
				IsPdf:         ext == ".pdf",
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(uploadedItems)
	}
}

func newUploadsServeHandler(
	uploadsDir string,
	jwtSecret string,
	validator middleware.SessionVersionValidator,
	goalLookup proof.UploadGoalLookup,
	proofRepo proof.ProofRepository,
) http.Handler {
	return newUploadsServeHandlerWithChat(uploadsDir, jwtSecret, validator, goalLookup, proofRepo, nil)
}

func newUploadsServeHandlerWithChat(
	uploadsDir string,
	jwtSecret string,
	validator middleware.SessionVersionValidator,
	goalLookup proof.UploadGoalLookup,
	proofRepo proof.ProofRepository,
	communityChecker proof.UploadCommunityChecker,
) http.Handler {
	_ = os.MkdirAll(filepath.Join(uploadsDir, "proofs"), 0700)
	_ = os.MkdirAll(filepath.Join(uploadsDir, "chat"), 0700)
	uploadFileServer := http.FileServer(http.Dir(uploadsDir))

	return middleware.Auth(jwtSecret, middleware.WithSessionValidator(validator))(
		http.StripPrefix("/uploads/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cleanPath := strings.TrimSpace(r.URL.Path)
			if strings.Contains(cleanPath, "..") || strings.Contains(cleanPath, "%2e") || strings.Contains(cleanPath, "\x00") || proof.HasHiddenUploadSegment(cleanPath) {
				http.Error(w, "invalid path", http.StatusBadRequest)
				return
			}

			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute))

			callerID, _ := middleware.GetUserID(r.Context())
			callerEmail, _ := middleware.GetUserEmail(r.Context())
			callerHandle, _ := middleware.GetUserHandle(r.Context())
			goalIDHint := r.URL.Query().Get("goalId")

			if !proof.CanAccessUploadWithChat(r.Context(), uploadsDir, cleanPath, goalIDHint, callerID, callerEmail, callerHandle, goalLookup, proofRepo, communityChecker) {
				http.Error(w, "forbidden: you are not authorized to access this upload", http.StatusForbidden)
				return
			}

			ext := strings.ToLower(filepath.Ext(cleanPath))
			spec, allowed := allowedUploadSpecs[ext]
			if !allowed {
				http.Error(w, "unsupported file type", http.StatusForbidden)
				return
			}

			if spec.isInline {
				w.Header().Set("Content-Disposition", "inline")
				// Note: Chromium's built-in PDF viewer requires no 'sandbox' CSP directive on inline PDF responses.
				w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; object-src 'self'; script-src 'none'; frame-ancestors 'self' http://localhost:* http://127.0.0.1:* https://onusly.com https://*.onusly.com https://onusly.app https://*.onusly.app https://onusly.duckdns.org;")
			} else {
				w.Header().Set("Content-Type", spec.contentType)
				w.Header().Set("Content-Disposition", "attachment")
				w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'none'; sandbox; frame-ancestors 'self' http://localhost:* http://127.0.0.1:* https://onusly.com https://*.onusly.com https://onusly.app https://*.onusly.app https://onusly.duckdns.org;")
			}
			w.Header().Set("Cache-Control", "private, max-age=3600")
			w.Header().Del("X-Frame-Options")
			uploadFileServer.ServeHTTP(w, r)
		})),
	)
}
