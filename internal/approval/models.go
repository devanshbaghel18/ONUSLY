package approval

import (
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
)

type Approval struct {
	PK string `dynamodbav:"PK" json:"-"`
	SK string `dynamodbav:"SK" json:"-"`

	ID         string `dynamodbav:"ID" json:"id"`
	GoalID     string `dynamodbav:"GoalID" json:"goalId"`
	ProofID    string `dynamodbav:"ProofID" json:"proofId"`
	OwnerID    string `dynamodbav:"OwnerID" json:"ownerId"`
	ApproverID string `dynamodbav:"ApproverID" json:"approverId"`
	Status     string `dynamodbav:"Status" json:"status"`
	Comment    string `dynamodbav:"Comment,omitempty" json:"comment,omitempty"`
	DecidedAt  string `dynamodbav:"DecidedAt,omitempty" json:"decidedAt,omitempty"`
}

// ApproverInboxItem represents an actionable review card for an assigned approver.
type ApproverInboxItem struct {
	GoalID             string                    `json:"goalId"`
	OwnerID            string                    `json:"ownerId"`
	OwnerName          string                    `json:"ownerName,omitempty"`
	OwnerHandle        string                    `json:"ownerHandle,omitempty"`
	OwnerEmail         string                    `json:"ownerEmail,omitempty"`
	Title              string                    `json:"title"`
	Description        string                    `json:"description"`
	Status             string                    `json:"status"`
	CreatedAt          string                    `json:"createdAt"`
	UpdatedAt          string                    `json:"updatedAt,omitempty"`
	BlockedApps        []string                  `json:"blockedApps,omitempty"`
	BlockedDomains     []string                  `json:"blockedDomains,omitempty"`
	ApplicationTargets []goals.ApplicationTarget `json:"applicationTargets,omitempty"`
	LatestProof        *proof.Proof              `json:"latestProof,omitempty"`
	ProofCount         int                       `json:"proofCount"`
}

// ApproverInboxResponse wraps inbox items with total count.
type ApproverInboxResponse struct {
	Items []ApproverInboxItem `json:"items"`
	Total int                 `json:"total"`
}

// SubmissionAttempt represents a single chronological attempt of proof submission and its decision.
type SubmissionAttempt struct {
	AttemptNumber int         `json:"attemptNumber"`
	ProofID       string      `json:"proofId"`
	SubmittedAt   string      `json:"submittedAt"`
	ProofType     string      `json:"proofType"`
	Explanation   string      `json:"explanation,omitempty"`
	ExternalLink  string      `json:"externalLink,omitempty"`
	PhotoURL      string      `json:"photoUrl,omitempty"`
	Files         interface{} `json:"files,omitempty"`
	Status        string      `json:"status"` // "pending" | "approved" | "rejected"
	ApprovalID    string      `json:"approvalId,omitempty"`
	ApproverID    string      `json:"approverId,omitempty"`
	ApproverName  string      `json:"approverName,omitempty"`
	Comment       string      `json:"comment,omitempty"`
	DecidedAt     string      `json:"decidedAt,omitempty"`
	IsLatest      bool        `json:"isLatest"`
}

// AccountabilityStats represents partner streak and verification velocity metrics.
type AccountabilityStats struct {
	CurrentStreak        int     `json:"currentStreak"`
	CompletionRate       float64 `json:"completionRate"`
	TotalVerified        int     `json:"totalVerified"`
	TotalRejected        int     `json:"totalRejected"`
	AvgReviewTimeMinutes int     `json:"avgReviewTimeMinutes"`
}

// AuditReceipt represents an immutable record of an accountability verification decision.
type AuditReceipt struct {
	ReceiptID    string `json:"receiptId"`
	GoalID       string `json:"goalId"`
	ProofID      string `json:"proofId"`
	AttemptIndex int    `json:"attemptIndex"`
	Decision     string `json:"decision"`
	DecidedAt    string `json:"decidedAt"`
	ApproverID   string `json:"approverId"`
	ApproverName string `json:"approverName,omitempty"`
	Comment      string `json:"comment,omitempty"`
}

// SubmissionDossier represents the full dossier of attempts, audit trail, and partnership streak metrics for a goal.
type SubmissionDossier struct {
	GoalID                string              `json:"goalId"`
	GoalTitle             string              `json:"goalTitle"`
	GoalStatus            string              `json:"goalStatus"`
	OwnerID               string              `json:"ownerId"`
	ApproverEmail         string              `json:"approverEmail,omitempty"`
	TotalAttempts         int                 `json:"totalAttempts"`
	PendingAttempt        *SubmissionAttempt  `json:"pendingAttempt,omitempty"`
	LatestDecision        *SubmissionAttempt  `json:"latestDecision,omitempty"`
	Attempts              []SubmissionAttempt `json:"attempts"`
	AuditLog              []AuditReceipt      `json:"auditLog"`
	AccountabilityMetrics AccountabilityStats `json:"accountabilityMetrics"`
}


