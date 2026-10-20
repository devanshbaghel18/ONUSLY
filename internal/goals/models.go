package goals

import "strings"

type Goal struct {
	PK string `dynamodbav:"PK" json:"-"`
	SK string `dynamodbav:"SK" json:"-"`

	ID             string   `dynamodbav:"ID" json:"id"`
	OwnerID        string   `dynamodbav:"OwnerID" json:"ownerId"`
	ApproverID     string   `dynamodbav:"ApproverID" json:"approverId"`
	ApproverEmail  string   `dynamodbav:"ApproverEmail,omitempty" json:"approverEmail,omitempty"`
	ApprovalType   string   `dynamodbav:"ApprovalType,omitempty" json:"approvalType,omitempty"`
	Title          string   `dynamodbav:"Title" json:"title"`
	Description    string   `dynamodbav:"Description" json:"description"`
	Status         string   `dynamodbav:"Status" json:"status"`
	CreatedAt      string   `dynamodbav:"CreatedAt" json:"createdAt"`
	UpdatedAt      string   `dynamodbav:"UpdatedAt,omitempty" json:"updatedAt,omitempty"`
	BlockedApps    []string `dynamodbav:"BlockedApps,omitempty" json:"blockedApps,omitempty"`
	BlockedDomains []string `dynamodbav:"BlockedDomains,omitempty" json:"blockedDomains,omitempty"`
}

type CreateGoalInput struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	ApprovalType   *string  `json:"approvalType"`
	ApproverEmail  *string  `json:"approverEmail"`
	BlockedApps    []string `json:"blockedApps"`
	BlockedDomains []string `json:"blockedDomains"`
}

type UpdateGoalInput struct {
	Title          *string
	Description    *string
	ApprovalType   *string
	ApproverEmail  *string
	ApproverID     *string
	UpdatedAt      string
	BlockedApps    *[]string
	BlockedDomains *[]string

	// Expected targets and conditions for atomic DynamoDB ConditionExpressions
	ExpectedBlockedApps         []string
	ExpectedBlockedDomains      []string
	CheckTargetsUnchanged       bool
	DisallowNoneIfLocked        bool
	RequireFriendAccountability bool
}

// IsApprover returns true if caller matches the goal's configured approver.
func (g *Goal) IsApprover(callerID, callerEmail, callerHandle string) bool {
	if g == nil {
		return false
	}
	if g.ApproverID == "COMMUNITY" || g.ApprovalType == "community" {
		return true
	}
	if g.ApproverID != "" && g.ApproverID == callerID {
		return true
	}
	if g.ApproverEmail != "" {
		if callerEmail != "" && strings.EqualFold(g.ApproverEmail, callerEmail) {
			return true
		}
		cleanHandle := strings.TrimPrefix(strings.TrimSpace(callerHandle), "@")
		if cleanHandle != "" {
			if strings.EqualFold(g.ApproverEmail, cleanHandle) || strings.EqualFold(g.ApproverEmail, "@"+cleanHandle) {
				return true
			}
		}
	}
	return false
}

// CanAccess returns true if the caller is either the owner or the designated approver.
func (g *Goal) CanAccess(callerID, callerEmail, callerHandle string) bool {
	if g == nil {
		return false
	}
	if g.OwnerID == callerID {
		return true
	}
	return g.IsApprover(callerID, callerEmail, callerHandle)
}
