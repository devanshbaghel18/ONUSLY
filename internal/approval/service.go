package approval

import (
	"context"
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/internal/events"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
	"github.com/google/uuid"
)

var (
	ErrInvalidDecision  = errors.New("invalid approval decision")
	ErrUnauthorized     = errors.New("user is not the assigned approver")
	ErrSelfApproval     = errors.New("goal owner cannot approve their own goal")
	ErrGoalNotFound     = errors.New("goal not found")
	ErrProofNotFound    = errors.New("proof not found")
	ErrInvalidProof     = errors.New("invalid proof")
	ErrAlreadyDecided   = errors.New("proof has already been decided")
	ErrApprovalNotFound = errors.New("approval not found")
	ErrCommentRequired  = errors.New("rejection requires a comment or reason")
)

type ApprovalRepository interface {
	Create(ctx context.Context, approval Approval) error
	CreateDecision(ctx context.Context, approval Approval, expectedGoalStatus string, newGoalStatus string) error
	GetByID(ctx context.Context, ownerID, goalID, approvalID string) (*Approval, error)
	ListByGoal(ctx context.Context, ownerID, goalID string) ([]Approval, error)
}

type Service struct {
	repo         ApprovalRepository
	goalService  *goals.Service
	proofService *proof.Service
	authRepo     auth.Repository
	listeners    []DecisionListener
	publisher    events.Publisher
}

func NewService(
	repo ApprovalRepository,
	goalService *goals.Service,
	proofService *proof.Service,
) *Service {
	return &Service{
		repo:         repo,
		goalService:  goalService,
		proofService: proofService,
		publisher:    &events.NoopPublisher{},
	}
}

// SetAuthRepo assigns an auth.Repository for enriching inbox items with owner profiles.
func (s *Service) SetAuthRepo(ar auth.Repository) {
	if ar != nil {
		s.authRepo = ar
	}
}

// SetPublisher assigns an events.Publisher for post-commit event dispatch.
func (s *Service) SetPublisher(p events.Publisher) {
	if p != nil {
		s.publisher = p
	}
}

// AddDecisionListener registers a listener to be notified after a decision is committed.
func (s *Service) AddDecisionListener(l DecisionListener) {
	if l != nil {
		s.listeners = append(s.listeners, l)
	}
}

func (s *Service) Decide(
	ctx context.Context,
	approverID string,
	ownerID string,
	goalID string,
	proofID string,
	status string,
	comment string,
) (*Approval, error) {

	approverID = strings.TrimSpace(approverID)
	ownerID = strings.TrimSpace(ownerID)
	goalID = strings.TrimSpace(goalID)
	proofID = strings.TrimSpace(proofID)
	status = strings.TrimSpace(status)
	comment = strings.TrimSpace(comment)

	if approverID == "" || goalID == "" || proofID == "" {
		return nil, errors.New("invalid approval request")
	}

	if status != "approved" && status != "rejected" {
		return nil, ErrInvalidDecision
	}

	if status == "rejected" && (comment == "" || len(strings.TrimSpace(comment)) < 5) {
		return nil, ErrCommentRequired
	}

	if len(comment) > 2000 {
		return nil, errors.New("comment exceeds maximum allowed length of 2000 characters")
	}

	goal, err := s.goalService.GetByID(ctx, ownerID, goalID)
	if err != nil || goal == nil {
		goal, err = s.goalService.GetByID(ctx, "", goalID)
		if err != nil || goal == nil {
			return nil, ErrGoalNotFound
		}
	}

	// 1. Goal owner must NEVER be able to approve their own goal.
	if goal.OwnerID == approverID {
		return nil, ErrSelfApproval
	}

	// 2. Goal must have a valid configured approver and caller must match.
	callerEmail, _ := middleware.GetUserEmail(ctx)
	callerHandle, _ := middleware.GetUserHandle(ctx)
	if !goal.IsApprover(approverID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	// 3. Goal must belong to the specified owner (if provided by caller, must not be forged).
	if ownerID != "" && goal.OwnerID != ownerID {
		return nil, ErrUnauthorized
	}
	ownerID = goal.OwnerID

	// 4. Goal must currently be in proof_submitted state.
	if goal.Status != "proof_submitted" {
		return nil, ErrAlreadyDecided
	}

	// 5. Proof must exist.
	p, err := s.proofService.GetByID(
		ctx,
		ownerID,
		goalID,
		proofID,
	)
	if err != nil {
		if errors.Is(err, proof.ErrProofNotFound) {
			return nil, ErrProofNotFound
		}
		return nil, ErrInvalidProof
	}

	// 6. Proof must belong to this specific goal and legitimate goal owner.
	if p.GoalID != goalID || p.OwnerID != goal.OwnerID || p.OwnerID != ownerID {
		return nil, ErrInvalidProof
	}

	approvalID := uuid.NewString()

	approval := Approval{
		PK:         "USER#" + goal.OwnerID,
		SK:         "APPROVAL#" + goalID + "#" + approvalID,
		ID:         approvalID,
		GoalID:     goalID,
		ProofID:    proofID,
		OwnerID:    goal.OwnerID,
		ApproverID: approverID,
		Status:     status,
		Comment:    comment,
		DecidedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	goalStatus := "rejected"
	if status == "approved" {
		goalStatus = "completed"
	}

	if err := s.repo.CreateDecision(
		ctx,
		approval,
		"proof_submitted",
		goalStatus,
	); err != nil {
		return nil, err
	}
	_, _ = s.goalService.UpdateStatus(ctx, goal.OwnerID, goalID, goalStatus)

	for _, listener := range s.listeners {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Approval] Recovered from listener panic: %v", r)
				}
			}()
			listener.OnDecision(ctx, &approval, goal)
		}()
	}

	if s.publisher != nil {
		decidedTime, _ := time.Parse(time.RFC3339, approval.DecidedAt)
		if decidedTime.IsZero() {
			decidedTime = time.Now().UTC()
		}
		event := events.ApprovalDecidedEvent{
			EventID:    "approval-" + approval.ID,
			GoalID:     approval.GoalID,
			ProofID:    approval.ProofID,
			OwnerID:    approval.OwnerID,
			ApproverID: approval.ApproverID,
			Status:     approval.Status,
			Comment:    approval.Comment,
			Title:      goal.Title,
			DecidedAt:  decidedTime,
		}
		_ = s.publisher.PublishApprovalDecided(ctx, event)
	}

	return &approval, nil
}

func (s *Service) GetByID(
	ctx context.Context,
	ownerID string,
	goalID string,
	approvalID string,
) (*Approval, error) {

	approval, err := s.repo.GetByID(
		ctx,
		ownerID,
		goalID,
		approvalID,
	)

	if err != nil {
		return nil, err
	}

	if approval == nil {
		return nil, errors.New("approval not found")
	}

	return approval, nil
}

// ListByGoalAuthorized validates that caller is goal owner or assigned approver before listing approvals.
func (s *Service) ListByGoalAuthorized(
	ctx context.Context,
	callerID, callerEmail, callerHandle, goalID string,
) ([]Approval, error) {
	goal, err := s.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	if !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	return s.repo.ListByGoal(ctx, goal.OwnerID, goalID)
}

// GetByIDAuthorized validates that caller is goal owner or assigned approver before returning approval details.
func (s *Service) GetByIDAuthorized(
	ctx context.Context,
	callerID, callerEmail, callerHandle, goalID, approvalID string,
) (*Approval, error) {
	goal, err := s.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	if !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	app, err := s.repo.GetByID(ctx, goal.OwnerID, goalID, approvalID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, ErrApprovalNotFound
	}
	if app.GoalID != goalID || app.OwnerID != goal.OwnerID {
		return nil, ErrApprovalNotFound
	}

	return app, nil
}

func (s *Service) ListByGoal(
	ctx context.Context,
	ownerID string,
	goalID string,
) ([]Approval, error) {

	return s.repo.ListByGoal(
		ctx,
		ownerID,
		goalID,
	)
}

// GetApproverInbox retrieves all goals waiting for review by the authenticated approver.
// It filters by status ("pending", "approved", "rejected", "all"), enriches each goal with
// the latest proof and owner profile details, and orders newest submissions first.
func (s *Service) GetApproverInbox(
	ctx context.Context,
	approverID string,
	approverEmail string,
	approverHandle string,
	statusFilter string,
) (*ApproverInboxResponse, error) {
	approverID = strings.TrimSpace(approverID)
	approverEmail = strings.ToLower(strings.TrimSpace(approverEmail))
	cleanHandle := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(approverHandle), "@"))
	statusFilter = strings.ToLower(strings.TrimSpace(statusFilter))

	if approverID == "" && approverEmail == "" && cleanHandle == "" {
		return nil, ErrUnauthorized
	}

	if statusFilter == "" {
		statusFilter = "pending"
	}

	if s.goalService == nil {
		return &ApproverInboxResponse{Items: []ApproverInboxItem{}, Total: 0}, nil
	}

	goalsList, err := s.goalService.ListForApproverWithStatus(ctx, approverID, approverEmail, cleanHandle, statusFilter)
	if err != nil {
		return nil, err
	}

	items := make([]ApproverInboxItem, 0, len(goalsList))

	for _, g := range goalsList {
		// Invariant: Goal owner cannot approve their own goal
		if approverID != "" && g.OwnerID == approverID {
			continue
		}

		// Verify caller is authorized approver
		if !g.IsApprover(approverID, approverEmail, cleanHandle) {
			continue
		}

		var latestProof *proof.Proof
		proofCount := 0

		if s.proofService != nil && g.OwnerID != "" && g.ID != "" {
			proofs, err := s.proofService.ListByGoal(ctx, g.OwnerID, g.ID)
			if err == nil && len(proofs) > 0 {
				proofCount = len(proofs)
				// Find most recent proof
				latest := proofs[len(proofs)-1]
				for _, p := range proofs {
					if p.SubmittedAt > latest.SubmittedAt {
						latest = p
					}
				}
				latestProof = &latest
			}
		}

		ownerName := ""
		ownerHandle := ""
		ownerEmail := ""

		if s.authRepo != nil && g.OwnerID != "" {
			ownerUser, err := s.authRepo.GetUserByID(ctx, g.OwnerID)
			if err == nil && ownerUser != nil {
				ownerName = ownerUser.Name
				ownerHandle = ownerUser.Handle
				ownerEmail = ownerUser.Email
			}
		}

		items = append(items, ApproverInboxItem{
			GoalID:             g.ID,
			OwnerID:            g.OwnerID,
			OwnerName:          ownerName,
			OwnerHandle:        ownerHandle,
			OwnerEmail:         ownerEmail,
			Title:              g.Title,
			Description:        g.Description,
			Status:             g.Status,
			CreatedAt:          g.CreatedAt,
			UpdatedAt:          g.UpdatedAt,
			BlockedApps:        g.BlockedApps,
			BlockedDomains:     g.BlockedDomains,
			ApplicationTargets: g.ApplicationTargets,
			LatestProof:        latestProof,
			ProofCount:         proofCount,
		})
	}

	// Sort items descending by submission time or creation time
	sort.SliceStable(items, func(i, j int) bool {
		timeI := items[i].CreatedAt
		if items[i].LatestProof != nil && items[i].LatestProof.SubmittedAt != "" {
			timeI = items[i].LatestProof.SubmittedAt
		} else if items[i].UpdatedAt != "" {
			timeI = items[i].UpdatedAt
		}

		timeJ := items[j].CreatedAt
		if items[j].LatestProof != nil && items[j].LatestProof.SubmittedAt != "" {
			timeJ = items[j].LatestProof.SubmittedAt
		} else if items[j].UpdatedAt != "" {
			timeJ = items[j].UpdatedAt
		}

		return timeI > timeJ
	})

	return &ApproverInboxResponse{
		Items: items,
		Total: len(items),
	}, nil
}

// GetSubmissions compiles the complete multi-attempt dossier, audit trail, and accountability streak metrics.
func (s *Service) GetSubmissions(
	ctx context.Context,
	callerID, callerEmail, callerHandle, goalID string,
) (*SubmissionDossier, error) {
	goal, err := s.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	if !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	proofs, err := s.proofService.ListByGoal(ctx, goal.OwnerID, goalID)
	if err != nil {
		return nil, err
	}

	approvals, err := s.repo.ListByGoal(ctx, goal.OwnerID, goalID)
	if err != nil {
		return nil, err
	}

	// Index approvals by ProofID
	approvalMap := make(map[string]Approval)
	for _, app := range approvals {
		if app.ProofID != "" {
			approvalMap[app.ProofID] = app
		}
	}

	// Sort proofs chronologically ascending (earliest attempt first)
	sort.SliceStable(proofs, func(i, j int) bool {
		if proofs[i].SubmittedAt == proofs[j].SubmittedAt {
			return proofs[i].ID < proofs[j].ID
		}
		return proofs[i].SubmittedAt < proofs[j].SubmittedAt
	})

	attempts := make([]SubmissionAttempt, 0, len(proofs))
	auditLog := make([]AuditReceipt, 0, len(approvals))
	totalReviewMinutes := 0
	reviewCount := 0

	for i, p := range proofs {
		attemptNum := i + 1
		isLatest := (i == len(proofs)-1)

		attempt := SubmissionAttempt{
			AttemptNumber: attemptNum,
			ProofID:       p.ID,
			SubmittedAt:   p.SubmittedAt,
			ProofType:     p.ProofType,
			Explanation:   p.TextExplanation,
			ExternalLink:  p.ExternalLink,
			PhotoURL:      p.PhotoURL,
			Files:         p.Files,
			Status:        "pending",
			IsLatest:      isLatest,
		}

		if app, found := approvalMap[p.ID]; found {
			attempt.Status = app.Status
			attempt.ApprovalID = app.ID
			attempt.ApproverID = app.ApproverID
			attempt.Comment = app.Comment
			attempt.DecidedAt = app.DecidedAt

			// Attempt to resolve approver name
			if s.authRepo != nil && app.ApproverID != "" {
				if approverUser, err := s.authRepo.GetUserByID(ctx, app.ApproverID); err == nil && approverUser != nil {
					attempt.ApproverName = approverUser.Name
					if attempt.ApproverName == "" {
						attempt.ApproverName = approverUser.Handle
					}
				}
			}

			// Add to audit trail
			auditLog = append(auditLog, AuditReceipt{
				ReceiptID:    app.ID,
				GoalID:       goalID,
				ProofID:      p.ID,
				AttemptIndex: attemptNum,
				Decision:     app.Status,
				DecidedAt:    app.DecidedAt,
				ApproverID:   app.ApproverID,
				ApproverName: attempt.ApproverName,
				Comment:      app.Comment,
			})

			// Calculate turnaround time if both timestamps exist
			if p.SubmittedAt != "" && app.DecidedAt != "" {
				subTime, err1 := time.Parse(time.RFC3339, p.SubmittedAt)
				decTime, err2 := time.Parse(time.RFC3339, app.DecidedAt)
				if err1 == nil && err2 == nil && decTime.After(subTime) {
					totalReviewMinutes += int(decTime.Sub(subTime).Minutes())
					reviewCount++
				}
			}
		}

		attempts = append(attempts, attempt)
	}

	// Calculate Accountability Metrics across user's goals
	stats := AccountabilityStats{}
	if reviewCount > 0 {
		stats.AvgReviewTimeMinutes = totalReviewMinutes / reviewCount
	}

	// Query user goals to compute active streak and completion rate
	userGoals, err := s.goalService.List(ctx, goal.OwnerID)
	if err == nil && len(userGoals) > 0 {
		completedCount := 0
		for _, g := range userGoals {
			if g.Status == "completed" {
				completedCount++
			}
		}
		stats.TotalVerified = completedCount
		stats.TotalRejected = len(auditLog) - completedCount
		if stats.TotalRejected < 0 {
			stats.TotalRejected = 0
		}
		stats.CompletionRate = float64(completedCount) / float64(len(userGoals)) * 100.0

		// Calculate consecutive streak
		streak := 0
		for _, g := range userGoals {
			if g.Status == "completed" {
				streak++
			} else if g.Status == "active" || g.Status == "proof_submitted" {
				// in progress, continue
				continue
			} else {
				break
			}
		}
		stats.CurrentStreak = streak
	}

	dossier := &SubmissionDossier{
		GoalID:                goalID,
		GoalTitle:             goal.Title,
		GoalStatus:            goal.Status,
		OwnerID:               goal.OwnerID,
		ApproverEmail:         goal.ApproverEmail,
		TotalAttempts:         len(attempts),
		Attempts:              attempts,
		AuditLog:              auditLog,
		AccountabilityMetrics: stats,
	}

	// Identify pending attempt and latest decision
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].Status == "pending" {
			dossier.PendingAttempt = &attempts[i]
		}
		if attempts[i].Status == "approved" || attempts[i].Status == "rejected" {
			if dossier.LatestDecision == nil {
				dossier.LatestDecision = &attempts[i]
			}
		}
	}

	return dossier, nil
}


