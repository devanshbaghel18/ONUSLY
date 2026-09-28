package auth

type User struct {
	PK        string `dynamodbav:"PK" json:"-"`
	SK        string `dynamodbav:"SK" json:"-"`
	GSI1PK    string `dynamodbav:"GSI1PK" json:"-"`
	GSI1SK    string `dynamodbav:"GSI1SK" json:"-"`

	ID        string `dynamodbav:"ID" json:"id"`
	Email     string `dynamodbav:"Email" json:"email"`
	Name      string `dynamodbav:"Name" json:"name"`
	Picture   string `dynamodbav:"Picture" json:"picture"`
	CreatedAt string `dynamodbav:"CreatedAt" json:"createdAt"`
}