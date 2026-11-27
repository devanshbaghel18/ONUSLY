package main

import (
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

func newUploadCreateHandler(uploadsDir string) http.HandlerFunc {
	_ = os.MkdirAll(filepath.Join(uploadsDir, "proofs"), 0700)

	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserID(r.Context())
		if !ok || userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}

		goalID := strings.TrimSpace(r.FormValue("goalId"))
		if goalID == "" {
			goalID = strings.TrimSpace(r.URL.Query().Get("goalId"))
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
			ext := strings.ToLower(filepath.Ext(fh.Filename))
			switch ext {
			case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".pdf", ".txt":
			default:
				http.Error(w, "unsupported file extension: allowed png, jpg, webp, gif, pdf, txt", http.StatusBadRequest)
				return
			}

			src, err := fh.Open()
			if err != nil {
				continue
			}

			// Sniff leading bytes to verify actual MIME content matches allowed proof types
			sniffBuf := make([]byte, 512)
			n, _ := io.ReadFull(src, sniffBuf)
			detectedMIME := strings.ToLower(http.DetectContentType(sniffBuf[:n]))
			mimeValid := false
			switch ext {
			case ".png":
				mimeValid = detectedMIME == "image/png"
			case ".jpg", ".jpeg":
				mimeValid = detectedMIME == "image/jpeg"
			case ".webp":
				mimeValid = detectedMIME == "image/webp"
			case ".gif":
				mimeValid = detectedMIME == "image/gif"
			case ".pdf":
				mimeValid = detectedMIME == "application/pdf"
			case ".txt":
				mimeValid = strings.HasPrefix(detectedMIME, "text/plain")
			}
			if !mimeValid {
				src.Close()
				http.Error(w, "file content does not match allowed extension MIME type", http.StatusBadRequest)
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
			destPath := filepath.Join(uploadsDir, "proofs", uniqueFileName)

			dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				src.Close()
				continue
			}

			if _, err := dst.Write(sniffBuf[:n]); err != nil {
				src.Close()
				dst.Close()
				_ = os.Remove(destPath)
				continue
			}

			copiedRest, err := io.Copy(dst, src)
			src.Close()
			dst.Close()
			if err != nil {
				_ = os.Remove(destPath)
				continue
			}
			copiedSize := int64(n) + copiedRest

			fileURL := fmt.Sprintf("/uploads/proofs/%s", uniqueFileName)
			_ = proof.RecordUploadOwner(uploadsDir, fileURL, userID, goalID)

			contentType := detectedMIME
			if ext == ".txt" {
				contentType = "text/plain"
			}

			formattedSize := fmt.Sprintf("%d KB", copiedSize/1024)
			if copiedSize > 1024*1024 {
				formattedSize = fmt.Sprintf("%.1f MB", float64(copiedSize)/(1024*1024))
			}

			uploadedItems = append(uploadedItems, UploadedFileItem{
				ID:            "file-" + uuid.New().String()[:8],
				Name:          fh.Filename,
				Size:          copiedSize,
				FormattedSize: formattedSize,
				Type:          contentType,
				URL:           fileURL,
				DataURL:       fileURL,
				IsImage:       strings.HasPrefix(contentType, "image/"),
				IsPdf:         contentType == "application/pdf",
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
	_ = os.MkdirAll(filepath.Join(uploadsDir, "proofs"), 0700)
	uploadFileServer := http.FileServer(http.Dir(uploadsDir))

	return middleware.Auth(jwtSecret, middleware.WithSessionValidator(validator))(
		http.StripPrefix("/uploads/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cleanPath := strings.TrimSpace(r.URL.Path)
			if strings.Contains(cleanPath, "..") || strings.Contains(cleanPath, "%2e") || strings.Contains(cleanPath, "\x00") || proof.HasHiddenUploadSegment(cleanPath) {
				http.Error(w, "invalid path", http.StatusBadRequest)
				return
			}

			callerID, _ := middleware.GetUserID(r.Context())
			callerEmail, _ := middleware.GetUserEmail(r.Context())
			callerHandle, _ := middleware.GetUserHandle(r.Context())
			goalIDHint := r.URL.Query().Get("goalId")

			if !proof.CanAccessUpload(r.Context(), uploadsDir, cleanPath, goalIDHint, callerID, callerEmail, callerHandle, goalLookup, proofRepo) {
				http.Error(w, "forbidden: you are not authorized to access this proof upload", http.StatusForbidden)
				return
			}

			ext := strings.ToLower(filepath.Ext(cleanPath))
			switch ext {
			case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".pdf":
				w.Header().Set("Content-Disposition", "inline")
			case ".txt":
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("Content-Disposition", "attachment")
			default:
				http.Error(w, "unsupported file type", http.StatusForbidden)
				return
			}
			w.Header().Set("Cache-Control", "private, max-age=3600")
			w.Header().Del("X-Frame-Options")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; object-src 'self'; script-src 'none'; sandbox; frame-ancestors 'self' http://localhost:* http://127.0.0.1:* https://onusly.com https://*.onusly.com https://onusly.app https://*.onusly.app https://onusly.duckdns.org;")
			uploadFileServer.ServeHTTP(w, r)
		})),
	)
}
