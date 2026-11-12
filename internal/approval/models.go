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

