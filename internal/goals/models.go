package goals

type Goal struct {
	PK          string `dynamodbav:"PK" json:"-"`
	SK          string `dynamodbav:"SK" json:"-"`

	ID          string `dynamodbav:"ID" json:"id"`
	OwnerID     string `dynamodbav:"OwnerID" json:"ownerId"`
	Title       string `dynamodbav:"Title" json:"title"`
	Description string `dynamodbav:"Description" json:"description"`
	Status      string `dynamodbav:"Status" json:"status"`
	CreatedAt   string `dynamodbav:"CreatedAt" json:"createdAt"`
}
