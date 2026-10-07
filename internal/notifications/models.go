package notifications

import (
	"errors"
)

var (
	ErrAlreadyExists        = errors.New("notification already exists")
	ErrNotificationNotFound = errors.New("notification not found")
	ErrUnauthorizedAccess   = errors.New("unauthorized access to notification")
)

// Notification represents an in-app notification stored in DynamoDB.
// Partition Key: PK = USER#<recipientID>
// Sort Key:      SK = NOTIF#<sourceEventID>
type Notification struct {
	PK          string `dynamodbav:"PK" json:"-"`
	SK          string `dynamodbav:"SK" json:"-"`
	ID          string `dynamodbav:"id" json:"id"`                   // Deterministic source event ID (e.g. proof-123 or approval-456)
	RecipientID string `dynamodbav:"recipientId" json:"recipientId"` // Target user who sees the notification
	Type        string `dynamodbav:"type" json:"type"`               // "proof_submitted" | "approval_decided"
	Title       string `dynamodbav:"title" json:"title"`
	Message     string `dynamodbav:"message" json:"message"`
	GoalID      string `dynamodbav:"goalId" json:"goalId"`
	ProofID     string `dynamodbav:"proofId" json:"proofId"`
	Read        bool   `dynamodbav:"read" json:"read"`
	CreatedAt   string `dynamodbav:"createdAt" json:"createdAt"` // RFC3339 timestamp
}

// NotificationListResponse represents the payload for GET /notifications.
type NotificationListResponse struct {
	Notifications []Notification `json:"notifications"`
	UnreadCount   int            `json:"unreadCount"`
}
