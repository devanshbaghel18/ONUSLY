package goals

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
	ExpectedBlockedApps    []string
	ExpectedBlockedDomains []string
	CheckTargetsUnchanged  bool
	DisallowNoneIfLocked   bool
}
