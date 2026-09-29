package approval

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
