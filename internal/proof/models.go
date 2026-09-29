package proof

type Proof struct {
	PK string `dynamodbav:"PK" json:"-"`
	SK string `dynamodbav:"SK" json:"-"`

	ID              string `dynamodbav:"ID" json:"id"`
	GoalID          string `dynamodbav:"GoalID" json:"goalId"`
	OwnerID         string `dynamodbav:"OwnerID" json:"ownerId"`
	ProofType       string `dynamodbav:"ProofType" json:"proofType"`
	TextExplanation string `dynamodbav:"TextExplanation,omitempty" json:"textExplanation,omitempty"`
	ExternalLink    string `dynamodbav:"ExternalLink,omitempty" json:"externalLink,omitempty"`
	PhotoURL        string `dynamodbav:"PhotoURL,omitempty" json:"photoUrl,omitempty"`
	SubmittedAt     string `dynamodbav:"SubmittedAt" json:"submittedAt"`
}
