package chat

type Message struct {
	PK              string   `dynamodbav:"PK" json:"-"`
	SK              string   `dynamodbav:"SK" json:"-"`
	ID              string   `dynamodbav:"ID" json:"id"`
	SenderEmail     string   `dynamodbav:"SenderEmail" json:"senderEmail"`
	SenderHandle    string   `dynamodbav:"SenderHandle,omitempty" json:"senderHandle,omitempty"`
	SenderID        string   `dynamodbav:"SenderID" json:"senderId"`
	RecipientEmail  string   `dynamodbav:"RecipientEmail" json:"recipientEmail"`
	RecipientHandle string   `dynamodbav:"RecipientHandle,omitempty" json:"recipientHandle,omitempty"`
	Text            string   `dynamodbav:"Text" json:"text"`
	IsProof         bool     `dynamodbav:"IsProof,omitempty" json:"isProof,omitempty"`
	GoalID          string   `dynamodbav:"GoalID,omitempty" json:"goalId,omitempty"`
	GoalTitle       string   `dynamodbav:"GoalTitle,omitempty" json:"goalTitle,omitempty"`
	OwnerID         string   `dynamodbav:"OwnerID,omitempty" json:"ownerId,omitempty"`
	Images          []string `dynamodbav:"Images,omitempty" json:"images,omitempty"`
	ExternalLink    string   `dynamodbav:"ExternalLink,omitempty" json:"externalLink,omitempty"`
	Approved        bool     `dynamodbav:"Approved,omitempty" json:"approved,omitempty"`
	Time            string   `dynamodbav:"Time" json:"time"`
	Delivered       bool     `dynamodbav:"Delivered" json:"delivered"`
}
