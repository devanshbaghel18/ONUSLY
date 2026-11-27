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

// UploadMetadata tracks the authenticated uploader and bound goal or chat target for an upload file.
type UploadMetadata struct {
	OwnerID     string `json:"ownerId"`
	Scope       string `json:"scope,omitempty"`
	GoalID      string `json:"goalId,omitempty"`
	RecipientID string `json:"recipientId,omitempty"`
	CommunityID string `json:"communityId,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

// UploadGoalLookup resolves a goal for upload read authorization checks.
type UploadGoalLookup interface {
	GetByID(ctx context.Context, ownerID, goalID string) (*goals.Goal, error)
}

// UploadCommunityChecker checks whether a user belongs to a community for chat attachment ACLs.
type UploadCommunityChecker interface {
	IsCommunityMember(ctx context.Context, userID, email, handle, communityID string) bool
}

func extractUploadScopeAndFilename(urlOrPath string) (string, string, bool) {
	trimmed := strings.TrimSpace(urlOrPath)
	if trimmed == "" {
		return "", "", false
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "http://") || strings.HasPrefix(strings.ToLower(trimmed), "https://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", "", false
		}
		trimmed = parsed.Path
	} else if idx := strings.IndexAny(trimmed, "?#"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
	for _, scope := range []string{"proofs", "chat"} {
		prefix := "/uploads/" + scope + "/"
		if strings.HasPrefix(cleaned, prefix) {
			name := strings.TrimPrefix(cleaned, prefix)
			if name == "" || strings.Contains(name, "/") || strings.HasPrefix(name, ".") {
				return "", "", false
			}
			return scope, name, true
		}
	}
	return "", "", false
}

func extractProofUploadFilename(urlOrPath string) (string, bool) {
	scope, name, ok := extractUploadScopeAndFilename(urlOrPath)
	if !ok || scope != "proofs" {
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

func uploadMetaFilePath(uploadsDir, scope, filename string) string {
	if scope == "" {
		scope = "proofs"
	}
	return filepath.Join(uploadsDir, scope, uploadMetaDirName, filename+".json")
}

func readUploadMetadata(uploadsDir, scope, filename string) (*UploadMetadata, error) {
	data, err := os.ReadFile(uploadMetaFilePath(uploadsDir, scope, filename))
	if err != nil {
		return nil, err
	}
	var meta UploadMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func writeUploadMetadata(uploadsDir, scope, filename string, meta UploadMetadata) error {
	if scope == "" {
		scope = "proofs"
	}
	metaDir := filepath.Join(uploadsDir, scope, uploadMetaDirName)
	if err := os.MkdirAll(metaDir, 0700); err != nil {
		return err
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(uploadMetaFilePath(uploadsDir, scope, filename), payload, 0600)
}

// RecordUploadOwner persists owner (and optional goal) metadata alongside a proof upload.
func RecordUploadOwner(uploadsDir, fileURL, ownerID, goalID string) error {
	scope, filename, ok := extractUploadScopeAndFilename(fileURL)
	if !ok || strings.TrimSpace(ownerID) == "" {
		return nil
	}
	return writeUploadMetadata(uploadsDir, scope, filename, UploadMetadata{
		OwnerID:   strings.TrimSpace(ownerID),
		Scope:     scope,
		GoalID:    strings.TrimSpace(goalID),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// RecordChatUploadMetadata persists sender and recipient/community metadata alongside a chat upload.
func RecordChatUploadMetadata(uploadsDir, fileURL, ownerID, recipientID, communityID string) error {
	scope, filename, ok := extractUploadScopeAndFilename(fileURL)
	if !ok || strings.TrimSpace(ownerID) == "" {
		return nil
	}
	return writeUploadMetadata(uploadsDir, scope, filename, UploadMetadata{
		OwnerID:     strings.TrimSpace(ownerID),
		Scope:       scope,
		RecipientID: strings.TrimSpace(recipientID),
		CommunityID: strings.TrimSpace(communityID),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	})
}

// VerifyUploadOwnership rejects attempts by another user to attach someone else's local proof upload URL.
func VerifyUploadOwnership(uploadsDir, fileURL, callerOwnerID string) error {
	scope, filename, ok := extractUploadScopeAndFilename(fileURL)
	if !ok {
		return nil
	}
	meta, err := readUploadMetadata(uploadsDir, scope, filename)
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
	scope, filename, ok := extractUploadScopeAndFilename(fileURL)
	if !ok || scope != "proofs" || strings.TrimSpace(ownerID) == "" || strings.TrimSpace(goalID) == "" {
		return nil
	}
	meta, err := readUploadMetadata(uploadsDir, scope, filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeUploadMetadata(uploadsDir, scope, filename, UploadMetadata{
				OwnerID:   strings.TrimSpace(ownerID),
				Scope:     scope,
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
	meta.Scope = scope
	meta.GoalID = strings.TrimSpace(goalID)
	return writeUploadMetadata(uploadsDir, scope, filename, *meta)
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
	return CanAccessUploadWithChat(ctx, uploadsDir, relPath, goalIDHint, callerID, callerEmail, callerHandle, goalLookup, proofRepo, nil)
}

// CanAccessUploadWithChat enforces fail-closed authorization for both proof and chat uploads.
func CanAccessUploadWithChat(
	ctx context.Context,
	uploadsDir string,
	relPath string,
	goalIDHint string,
	callerID string,
	callerEmail string,
	callerHandle string,
	goalLookup UploadGoalLookup,
	proofRepo ProofRepository,
	communityChecker UploadCommunityChecker,
) bool {
	if strings.TrimSpace(callerID) == "" || HasHiddenUploadSegment(relPath) {
		return false
	}

	cleaned := path.Clean("/uploads/" + strings.TrimPrefix(strings.ReplaceAll(relPath, "\\", "/"), "/"))
	scope, filename, ok := extractUploadScopeAndFilename(cleaned)
	if !ok {
		return false
	}

	if meta, err := readUploadMetadata(uploadsDir, scope, filename); err == nil && meta != nil {
		if meta.OwnerID != "" && meta.OwnerID == callerID {
			return true
		}
		if scope == "chat" || meta.Scope == "chat" {
			if meta.RecipientID != "" && meta.RecipientID == callerID {
				return true
			}
			if meta.CommunityID != "" && communityChecker != nil && communityChecker.IsCommunityMember(ctx, callerID, callerEmail, callerHandle, meta.CommunityID) {
				return true
			}
			return false
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

	if scope != "proofs" {
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
