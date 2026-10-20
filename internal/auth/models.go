package auth

type User struct {
	PK     string `dynamodbav:"PK" json:"-"`
	SK     string `dynamodbav:"SK" json:"-"`
	GSI1PK string `dynamodbav:"GSI1PK" json:"-"`
	GSI1SK string `dynamodbav:"GSI1SK" json:"-"`

	ID        string `dynamodbav:"ID" json:"id"`
	Email     string `dynamodbav:"Email" json:"email"`
	Handle    string `dynamodbav:"Handle" json:"handle"`
	Name      string `dynamodbav:"Name" json:"name"`
	Picture   string `dynamodbav:"Picture" json:"picture"`
	CreatedAt string `dynamodbav:"CreatedAt" json:"createdAt"`
}

type PublicProfile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Handle  string `json:"handle"`
	Picture string `json:"picture"`
}

// HandleReservation ensures atomic exclusivity for public @handles in DynamoDB.
// Partition key is HANDLE#<clean_handle>, Sort key is RESERVATION.
type HandleReservation struct {
	PK        string `dynamodbav:"PK"`
	SK        string `dynamodbav:"SK"`
	UserID    string `dynamodbav:"UserID"`
	Handle    string `dynamodbav:"Handle"`
	CreatedAt string `dynamodbav:"CreatedAt"`
}
