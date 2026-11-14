package goals

import "strings"

// ApplicationTarget defines an explicitly typed enforcement target.
type ApplicationTarget struct {
	Kind       string `dynamodbav:"Kind" json:"kind"` // "desktop_app" | "android_package"
	Identifier string `dynamodbav:"Identifier" json:"identifier"`
}

type Goal struct {
	PK string `dynamodbav:"PK" json:"-"`
	SK string `dynamodbav:"SK" json:"-"`

	// GSI1 for efficient indexed global lookup
	GSI1PK string `dynamodbav:"GSI1PK,omitempty" json:"-"`
	GSI1SK string `dynamodbav:"GSI1SK,omitempty" json:"-"`

	// GSI2 for efficient approver inbox indexed lookup
	GSI2PK string `dynamodbav:"GSI2PK,omitempty" json:"-"`
	GSI2SK string `dynamodbav:"GSI2SK,omitempty" json:"-"`

	ID                 string              `dynamodbav:"ID" json:"id"`
	OwnerID            string              `dynamodbav:"OwnerID" json:"ownerId"`
	ApproverID         string              `dynamodbav:"ApproverID" json:"approverId"`
	ApproverEmail      string              `dynamodbav:"ApproverEmail,omitempty" json:"approverEmail,omitempty"`
	ApprovalType       string              `dynamodbav:"ApprovalType,omitempty" json:"approvalType,omitempty"`
	Title              string              `dynamodbav:"Title" json:"title"`
	Description        string              `dynamodbav:"Description" json:"description"`
	Status             string              `dynamodbav:"Status" json:"status"`
	Version            int64               `dynamodbav:"Version,omitempty" json:"version,omitempty"`
	CreatedAt          string              `dynamodbav:"CreatedAt" json:"createdAt"`
	UpdatedAt          string              `dynamodbav:"UpdatedAt,omitempty" json:"updatedAt,omitempty"`
	BlockedApps        []string            `dynamodbav:"BlockedApps,omitempty" json:"blockedApps,omitempty"`
	BlockedDomains     []string            `dynamodbav:"BlockedDomains,omitempty" json:"blockedDomains,omitempty"`
	ApplicationTargets []ApplicationTarget `dynamodbav:"ApplicationTargets,omitempty" json:"applicationTargets,omitempty"`
}

type CreateGoalInput struct {
	Title              string              `json:"title"`
	Description        string              `json:"description"`
	ApprovalType       *string             `json:"approvalType"`
	ApproverEmail      *string             `json:"approverEmail"`
	BlockedApps        []string            `json:"blockedApps"`
	BlockedDomains     []string            `json:"blockedDomains"`
	ApplicationTargets []ApplicationTarget `json:"applicationTargets,omitempty"`
}

type UpdateGoalInput struct {
	Title              *string
	Description        *string
	ApprovalType       *string
	ApproverEmail      *string
	ApproverID         *string
	UpdatedAt          string
	BlockedApps        *[]string
	BlockedDomains     *[]string
	ApplicationTargets *[]ApplicationTarget

	// Optimistic concurrency control fields
	ExpectedVersion   *int64
	ExpectedUpdatedAt *string

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
	if g.ApproverID != "" {
		cleanAppID := strings.TrimPrefix(strings.ToLower(g.ApproverID), "partner-")
		if g.ApproverID == callerID || cleanAppID == strings.ToLower(callerID) {
			return true
		}
		cleanHandle := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(callerHandle)), "@")
		if cleanHandle != "" && (cleanAppID == cleanHandle || g.ApproverID == cleanHandle) {
			return true
		}
	}
	if g.ApproverEmail != "" {
		if callerEmail != "" && strings.EqualFold(g.ApproverEmail, callerEmail) {
			return true
		}
		cleanHandle := strings.TrimPrefix(strings.TrimSpace(callerHandle), "@")
		if cleanHandle != "" {
			if strings.EqualFold(g.ApproverEmail, cleanHandle) || strings.EqualFold(g.ApproverEmail, "@"+cleanHandle) || strings.EqualFold(g.ApproverEmail, "partner-"+cleanHandle) {
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

// FormatApproverGSI2PK formats the GSI2 partition key for an approver identifier, email, or handle.
func FormatApproverGSI2PK(approver string) string {
	clean := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(approver), "@"))
	return "APPROVER#" + clean
}

// FormatApproverGSI2SK formats the GSI2 sort key encoding goal status and goal ID.
func FormatApproverGSI2SK(status, goalID string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		s = "active"
	}
	return "STATUS#" + s + "#GOAL#" + strings.TrimSpace(goalID)
}

