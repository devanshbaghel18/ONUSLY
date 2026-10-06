package chat

type Message struct {
	PK              string `dynamodbav:"PK" json:"-"`
	SK              string `dynamodbav:"SK" json:"-"`
	ID              string `dynamodbav:"ID" json:"id"`
	SenderEmail     string `dynamodbav:"SenderEmail" json:"senderEmail"`
	SenderHandle    string `dynamodbav:"SenderHandle,omitempty" json:"senderHandle,omitempty"`
	SenderID        string `dynamodbav:"SenderID" json:"senderId"`
	RecipientEmail  string `dynamodbav:"RecipientEmail" json:"recipientEmail"`
	RecipientHandle string `dynamodbav:"RecipientHandle,omitempty" json:"recipientHandle,omitempty"`
	Text            string `dynamodbav:"Text" json:"text"`
	Time            string `dynamodbav:"Time" json:"time"`
	Delivered       bool   `dynamodbav:"Delivered" json:"delivered"`
}
