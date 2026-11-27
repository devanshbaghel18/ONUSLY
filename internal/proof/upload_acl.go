package proof

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

const uploadMetaDirName = ".meta"

// UploadMetadata tracks the authenticated uploader and bound goal for a proof upload file.
type UploadMetadata struct {
	OwnerID   string `json:"ownerId"`
	GoalID    string `json:"goalId,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// UploadGoalLookup resolves a goal for upload read authorization checks.
type UploadGoalLookup interface {
	GetByID(ctx context.Context, ownerID, goalID string) (*goals.Goal, error)
}

func extractProofUploadFilename(urlOrPath string) (string, bool) {
	trimmed := strings.TrimSpace(urlOrPath)
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "http://") || strings.HasPrefix(strings.ToLower(trimmed), "https://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", false
		}
		trimmed = parsed.Path
	} else if idx := strings.IndexAny(trimmed, "?#"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
	const prefix = "/uploads/proofs/"
	if !strings.HasPrefix(cleaned, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(cleaned, prefix)
	if name == "" || strings.Contains(name, "/") || strings.HasPrefix(name, ".") {
		return "", false
	}
	return name, true
}

func extractUploadedFileURLs(files interface{}) []string {
	var urls []string
	switch typed := files.(type) {
	case []interface{}:
		for _, item := range typed {
			if fm, ok := item.(map[string]interface{}); ok {
				if u, ok := fm["url"].(string); ok && strings.TrimSpace(u) != "" {
					urls = append(urls, u)
				}
				if du, ok := fm["dataUrl"].(string); ok && strings.TrimSpace(du) != "" {
					urls = append(urls, du)
				}
			}
		}
	case []map[string]interface{}:
		for _, fm := range typed {
			if u, ok := fm["url"].(string); ok && strings.TrimSpace(u) != "" {
				urls = append(urls, u)
			}
			if du, ok := fm["dataUrl"].(string); ok && strings.TrimSpace(du) != "" {
				urls = append(urls, du)
			}
		}
	}
	return urls
}

func HasHiddenUploadSegment(cleanPath string) bool {
	trimmed := strings.Trim(strings.ReplaceAll(cleanPath, "\\", "/"), "/")
	if trimmed == "" {
		return true
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == "" || strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func uploadMetaFilePath(uploadsDir, filename string) string {
	return filepath.Join(uploadsDir, "proofs", uploadMetaDirName, filename+".json")
}

func readUploadMetadata(uploadsDir, filename string) (*UploadMetadata, error) {
	data, err := os.ReadFile(uploadMetaFilePath(uploadsDir, filename))
	if err != nil {
		return nil, err
	}
	var meta UploadMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func writeUploadMetadata(uploadsDir, filename string, meta UploadMetadata) error {
	metaDir := filepath.Join(uploadsDir, "proofs", uploadMetaDirName)
	if err := os.MkdirAll(metaDir, 0700); err != nil {
		return err
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(uploadMetaFilePath(uploadsDir, filename), payload, 0600)
}

// RecordUploadOwner persists owner (and optional goal) metadata alongside a proof upload.
func RecordUploadOwner(uploadsDir, fileURL, ownerID, goalID string) error {
	filename, ok := extractProofUploadFilename(fileURL)
	if !ok || strings.TrimSpace(ownerID) == "" {
		return nil
	}
	return writeUploadMetadata(uploadsDir, filename, UploadMetadata{
		OwnerID:   strings.TrimSpace(ownerID),
		GoalID:    strings.TrimSpace(goalID),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// VerifyUploadOwnership rejects attempts by another user to attach someone else's local proof upload URL.
func VerifyUploadOwnership(uploadsDir, fileURL, callerOwnerID string) error {
	filename, ok := extractProofUploadFilename(fileURL)
	if !ok {
		return nil
	}
	meta, err := readUploadMetadata(uploadsDir, filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if meta.OwnerID != "" && meta.OwnerID != strings.TrimSpace(callerOwnerID) {
		return ErrUnauthorized
	}
	return nil
}

// BindUploadGoal binds an uploaded proof file to a goal so the assigned approver may read it.
func BindUploadGoal(uploadsDir, fileURL, ownerID, goalID string) error {
	filename, ok := extractProofUploadFilename(fileURL)
	if !ok || strings.TrimSpace(ownerID) == "" || strings.TrimSpace(goalID) == "" {
		return nil
	}
	meta, err := readUploadMetadata(uploadsDir, filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeUploadMetadata(uploadsDir, filename, UploadMetadata{
				OwnerID:   strings.TrimSpace(ownerID),
				GoalID:    strings.TrimSpace(goalID),
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			})
		}
		return err
	}
	if meta.OwnerID != "" && meta.OwnerID != strings.TrimSpace(ownerID) {
		return ErrUnauthorized
	}
	meta.OwnerID = strings.TrimSpace(ownerID)
	meta.GoalID = strings.TrimSpace(goalID)
	return writeUploadMetadata(uploadsDir, filename, *meta)
}

// CanAccessUpload enforces fail-closed authorization for GET /uploads/... requests.
// Only the uploader or an authorized goal approver/owner may fetch proof media.
func CanAccessUpload(
	ctx context.Context,
	uploadsDir string,
	relPath string,
	goalIDHint string,
	callerID string,
	callerEmail string,
	callerHandle string,
	goalLookup UploadGoalLookup,
	proofRepo ProofRepository,
) bool {
	if strings.TrimSpace(callerID) == "" || HasHiddenUploadSegment(relPath) {
		return false
	}

	cleaned := path.Clean("/uploads/" + strings.TrimPrefix(strings.ReplaceAll(relPath, "\\", "/"), "/"))
	filename, ok := extractProofUploadFilename(cleaned)
	if !ok {
		return false
	}

	if meta, err := readUploadMetadata(uploadsDir, filename); err == nil && meta != nil {
		if meta.OwnerID != "" && meta.OwnerID == callerID {
			return true
		}
		targetGoalID := strings.TrimSpace(meta.GoalID)
		if targetGoalID == "" {
			targetGoalID = strings.TrimSpace(goalIDHint)
		}
		if targetGoalID != "" && goalLookup != nil {
			goal, gErr := goalLookup.GetByID(ctx, meta.OwnerID, targetGoalID)
			if gErr == nil && goal != nil && goal.CanAccess(callerID, callerEmail, callerHandle) {
				return true
			}
		}
		return false
	}

	// Legacy files uploaded before ACL metadata existed require an explicit goalId hint
	// whose stored proof references this exact file and authorizes the caller.
	targetGoalID := strings.TrimSpace(goalIDHint)
	if targetGoalID == "" || goalLookup == nil || proofRepo == nil {
		return false
	}
	goal, err := goalLookup.GetByID(ctx, "", targetGoalID)
	if err != nil || goal == nil || !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return false
	}
	proofs, err := proofRepo.ListByGoal(ctx, goal.OwnerID, targetGoalID)
	if err != nil {
		return false
	}
	expectedURL := "/uploads/proofs/" + filename
	for _, p := range proofs {
		if strings.EqualFold(strings.TrimSpace(p.PhotoURL), expectedURL) {
			return true
		}
		if fileList, ok := p.Files.([]interface{}); ok {
			for _, item := range fileList {
				if fm, isMap := item.(map[string]interface{}); isMap {
					if u, _ := fm["url"].(string); strings.EqualFold(strings.TrimSpace(u), expectedURL) {
						return true
					}
					if du, _ := fm["dataUrl"].(string); strings.EqualFold(strings.TrimSpace(du), expectedURL) {
						return true
					}
				}
			}
		}
	}
	return false
}
