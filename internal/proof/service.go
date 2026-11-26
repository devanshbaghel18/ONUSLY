package proof

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/events"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/google/uuid"
)

var (
	ErrInvalidProofType      = errors.New("invalid proof type")
	ErrInvalidProof          = errors.New("invalid proof")
	ErrProofNotFound         = errors.New("proof not found")
	ErrGoalNotFound          = errors.New("goal not found")
	ErrProofAlreadySubmitted = errors.New("proof has already been submitted for this goal")
	ErrGoalNotActive         = errors.New("goal is not active")
	ErrUnauthorized          = errors.New("forbidden: you are not authorized to access proofs for this goal")
)

type ProofRepository interface {
	Create(ctx context.Context, p Proof) error
	CreateWithGoalTransition(ctx context.Context, p Proof, expectedGoalStatuses []string, newGoalStatus string) error
	GetByID(ctx context.Context, ownerID, goalID, proofID string) (*Proof, error)
	ListByGoal(ctx context.Context, ownerID, goalID string) ([]Proof, error)
}

type Service struct {
	repo        ProofRepository
	goalService *goals.Service
	publisher   events.Publisher
}

func NewService(repo ProofRepository, goalService *goals.Service) *Service {
	return &Service{
		repo:        repo,
		goalService: goalService,
		publisher:   &events.NoopPublisher{},
	}
}

// SetPublisher assigns an events.Publisher for post-commit event dispatch.
func (s *Service) SetPublisher(p events.Publisher) {
	if p != nil {
		s.publisher = p
	}
}

const maxProofUploadBytes = 10 * 1024 * 1024 // 10MB decoded ceiling per proof upload

func saveBase64ToFile(dataURI, prefix string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(dataURI), "data:") {
		return dataURI, nil
	}

	commaIdx := strings.Index(dataURI, ",")
	if commaIdx == -1 {
		return "", errors.New("malformed data URI")
	}

	header := strings.ToLower(dataURI[:commaIdx])
	if !strings.Contains(header, ";base64") {
		return "", errors.New("data URI must be base64-encoded")
	}
	payload := dataURI[commaIdx+1:]
	if len(payload) > (maxProofUploadBytes*4/3)+4 {
		return "", errors.New("uploaded proof file exceeds 10MB limit")
	}

	rawBytes, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", errors.New("invalid base64 proof payload")
	}
	if len(rawBytes) == 0 {
		return "", errors.New("empty proof payload")
	}
	if len(rawBytes) > maxProofUploadBytes {
		return "", errors.New("uploaded proof file exceeds 10MB limit")
	}

	detectedType := strings.ToLower(http.DetectContentType(rawBytes))
	ext := ""
	switch {
	case (strings.Contains(header, "image/jpeg") || strings.Contains(header, "image/jpg")) && detectedType == "image/jpeg":
		ext = ".jpg"
	case strings.Contains(header, "image/png") && detectedType == "image/png":
		ext = ".png"
	case strings.Contains(header, "image/webp") && detectedType == "image/webp":
		ext = ".webp"
	case strings.Contains(header, "image/gif") && detectedType == "image/gif":
		ext = ".gif"
	case strings.Contains(header, "application/pdf") && detectedType == "application/pdf":
		ext = ".pdf"
	case strings.Contains(header, "text/plain") && strings.HasPrefix(detectedType, "text/plain"):
		ext = ".txt"
	default:
		return "", errors.New("unsupported or mismatched proof file MIME type")
	}

	uploadsDir := filepath.Join("uploads", "proofs")
	_ = os.MkdirAll(uploadsDir, 0755)

	safePrefix := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, prefix)
	if safePrefix == "" {
		safePrefix = "proof"
	}

	filename := fmt.Sprintf("%d-%s-%s%s", time.Now().Unix(), uuid.New().String()[:8], safePrefix, ext)
	filePath := filepath.Join(uploadsDir, filename)

	if err := os.WriteFile(filePath, rawBytes, 0644); err != nil {
		return "", err
	}

	return "/uploads/proofs/" + filename, nil
}

func (s *Service) Submit(
	ctx context.Context,
	ownerID string,
	goalID string,
	proofType string,
	textExplanation string,
	externalLink string,
	photoURL string,
) (*Proof, error) {
	return s.SubmitWithFiles(ctx, ownerID, goalID, proofType, textExplanation, externalLink, photoURL, nil)
}

func (s *Service) SubmitWithFiles(
	ctx context.Context,
	ownerID string,
	goalID string,
	proofType string,
	textExplanation string,
	externalLink string,
	photoURL string,
	files interface{},
) (*Proof, error) {

	if strings.TrimSpace(ownerID) == "" {
		return nil, ErrInvalidProof
	}

	if strings.TrimSpace(goalID) == "" {
		return nil, ErrInvalidProof
	}

	proofType = strings.TrimSpace(proofType)

	switch proofType {
	case "text", "link", "photo", "document":
	default:
		return nil, ErrInvalidProofType
	}

	textExplanation = strings.TrimSpace(textExplanation)
	externalLink = strings.TrimSpace(externalLink)
	photoURL = strings.TrimSpace(photoURL)

	if len(textExplanation) > 5000 {
		return nil, errors.New("proof explanation exceeds maximum length of 5000 characters")
	}

	switch proofType {
	case "text":
		if textExplanation == "" {
			return nil, ErrInvalidProof
		}
	case "link":
		if externalLink == "" {
			return nil, ErrInvalidProof
		}
		if len(externalLink) > 2048 {
			return nil, errors.New("external link exceeds maximum length of 2048 characters")
		}
		parsedURL, err := url.ParseRequestURI(externalLink)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			return nil, errors.New("external link must be a valid http:// or https:// URL")
		}
	case "photo", "document":
		if photoURL == "" && textExplanation == "" {
			return nil, ErrInvalidProof
		}
		if photoURL != "" {
			lower := strings.ToLower(photoURL)
			if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "/uploads/") {
				if len(photoURL) > 2048 {
					return nil, errors.New("photo URL exceeds maximum length of 2048 characters")
				}
				if !strings.HasPrefix(lower, "/uploads/") {
					parsedURL, err := url.ParseRequestURI(photoURL)
					if err != nil || parsedURL.Host == "" {
						return nil, errors.New("photo URL must be a valid http(s) URL")
					}
				}
			} else if strings.HasPrefix(lower, "data:") {
				if proofType == "photo" {
					if len(photoURL) > 2*1024*1024 {
						return nil, errors.New("photo data URI exceeds maximum size limit of 2MB")
					}
					validMIMEPrefixes := []string{
						"data:image/jpeg;base64,",
						"data:image/jpg;base64,",
						"data:image/png;base64,",
						"data:image/webp;base64,",
						"data:image/gif;base64,",
						"data:application/pdf;base64,",
					}
					hasValidPrefix := false
					for _, prefix := range validMIMEPrefixes {
						if strings.HasPrefix(lower, prefix) {
							hasValidPrefix = true
							break
						}
					}
					if !hasValidPrefix {
						return nil, errors.New("photo data URI must have a valid MIME prefix (jpeg, png, webp, gif, pdf)")
					}
				} else if proofType == "document" {
					if len(photoURL) > 32*1024*1024 {
						return nil, errors.New("document data URI exceeds maximum size limit of 32MB")
					}
					if strings.HasPrefix(lower, "data:text/html") || strings.HasPrefix(lower, "data:application/x-msdownload") || strings.HasPrefix(lower, "data:application/javascript") {
						return nil, errors.New("document data URI has unsupported MIME type")
					}
				}
			} else {
				return nil, errors.New("photo URL must be an http(s) URL or valid image data URI")
			}
		}
	}

	// Auto-offload large base64 data to disk in uploads/proofs to stay well below DynamoDB's 400KB limit
	if strings.HasPrefix(strings.ToLower(photoURL), "data:") {
		savedURL, err := saveBase64ToFile(photoURL, "proof")
		if err != nil {
			return nil, err
		}
		photoURL = savedURL
	}

	if fileList, ok := files.([]interface{}); ok {
		for _, item := range fileList {
			if fileMap, isMap := item.(map[string]interface{}); isMap {
				if dUrl, hasDUrl := fileMap["dataUrl"].(string); hasDUrl && strings.HasPrefix(strings.ToLower(dUrl), "data:") {
					nameStr, _ := fileMap["name"].(string)
					saved, err := saveBase64ToFile(dUrl, nameStr)
					if err != nil {
						return nil, err
					}
					fileMap["dataUrl"] = saved
					fileMap["url"] = saved
				}
			}
		}
	}

	// Verify that the goal belongs to the authenticated user and is in "active" state.
	goal, err := s.goalService.GetByID(ctx, ownerID, goalID)
	if err != nil {
		return nil, ErrGoalNotFound
	}

	if goal.Status == "proof_submitted" {
		return nil, ErrProofAlreadySubmitted
	}

	if goal.Status != "active" && goal.Status != "rejected" {
		return nil, ErrGoalNotActive
	}

	proofID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	p := Proof{
		PK:              "USER#" + ownerID,
		SK:              "PROOF#" + goalID + "#" + proofID,
		ID:              proofID,
		GoalID:          goalID,
		OwnerID:         ownerID,
		ProofType:       proofType,
		TextExplanation: strings.TrimSpace(textExplanation),
		ExternalLink:    strings.TrimSpace(externalLink),
		PhotoURL:        strings.TrimSpace(photoURL),
		Files:           files,
		SubmittedAt:     now,
	}

	if err := s.repo.CreateWithGoalTransition(
		ctx,
		p,
		[]string{"active", "rejected"},
		"proof_submitted",
	); err != nil {
		return nil, err
	}

	if s.publisher != nil {
		submittedTime, _ := time.Parse(time.RFC3339, now)
		if submittedTime.IsZero() {
			submittedTime = time.Now().UTC()
		}
		approverTarget := goal.ApproverID
		if approverTarget == "" {
			approverTarget = goal.ApproverEmail
		}
		event := events.ProofSubmittedEvent{
			EventID:     "proof-" + p.ID,
			GoalID:      p.GoalID,
			ProofID:     p.ID,
			OwnerID:     p.OwnerID,
			ApproverID:  approverTarget,
			Title:       goal.Title,
			Files:       files,
			SubmittedAt: submittedTime,
		}
		_ = s.publisher.PublishProofSubmitted(ctx, event)
	}

	return &p, nil
}

func (s *Service) GetByID(
	ctx context.Context,
	ownerID string,
	goalID string,
	proofID string,
) (*Proof, error) {

	p, err := s.repo.GetByID(ctx, ownerID, goalID, proofID)
	if err != nil {
		return nil, err
	}

	if p == nil {
		return nil, ErrProofNotFound
	}

	return p, nil
}

// GetByIDAuthorized enforces that only the goal owner or designated approver can view a proof.
func (s *Service) GetByIDAuthorized(
	ctx context.Context,
	callerID string,
	callerEmail string,
	callerHandle string,
	goalID string,
	proofID string,
) (*Proof, error) {
	goal, err := s.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	if !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	p, err := s.repo.GetByID(ctx, goal.OwnerID, goalID, proofID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrProofNotFound
	}
	if p.GoalID != goalID || p.OwnerID != goal.OwnerID {
		return nil, ErrProofNotFound
	}

	return p, nil
}

// ListByGoalAuthorized enforces that only the goal owner or designated approver can list proofs.
func (s *Service) ListByGoalAuthorized(
	ctx context.Context,
	callerID string,
	callerEmail string,
	callerHandle string,
	goalID string,
) ([]Proof, error) {
	goal, err := s.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	if !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	return s.repo.ListByGoal(ctx, goal.OwnerID, goalID)
}

func (s *Service) ListByGoal(
	ctx context.Context,
	ownerID string,
	goalID string,
) ([]Proof, error) {
	if ownerID == "" {
		g, err := s.goalService.GetByID(ctx, "", goalID)
		if err == nil && g != nil {
			ownerID = g.OwnerID
		}
	}

	proofs, err := s.repo.ListByGoal(ctx, ownerID, goalID)
	if (err != nil || len(proofs) == 0) && goalID != "" {
		g, gErr := s.goalService.GetByID(ctx, "", goalID)
		if gErr == nil && g != nil && g.OwnerID != ownerID {
			return s.repo.ListByGoal(ctx, g.OwnerID, goalID)
		}
	}

	return proofs, err
}
