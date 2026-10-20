package friends

import "time"

// Friend represents an established mutual friendship.
type Friend struct {
	UserID       string    `dynamodbav:"UserID" json:"userId"`
	FriendID     string    `dynamodbav:"FriendID" json:"friendId"`
	FriendHandle string    `dynamodbav:"FriendHandle" json:"friendHandle"`
	FriendEmail  string    `dynamodbav:"FriendEmail" json:"friendEmail"`
	FriendName   string    `dynamodbav:"FriendName" json:"friendName"`
	CreatedAt    time.Time `dynamodbav:"CreatedAt" json:"createdAt"`
}

// FriendRequest represents a pending, accepted, or rejected friend request.
type FriendRequest struct {
	ID              string    `dynamodbav:"ID" json:"id"`
	SenderID        string    `dynamodbav:"SenderID" json:"senderId"`
	SenderHandle    string    `dynamodbav:"SenderHandle" json:"senderHandle"`
	SenderEmail     string    `dynamodbav:"SenderEmail" json:"senderEmail"`
	SenderName      string    `dynamodbav:"SenderName" json:"senderName"`
	RecipientID     string    `dynamodbav:"RecipientID" json:"recipientId"`
	RecipientHandle string    `dynamodbav:"RecipientHandle" json:"recipientHandle"`
	RecipientEmail  string    `dynamodbav:"RecipientEmail" json:"recipientEmail"`
	RecipientName   string    `dynamodbav:"RecipientName" json:"recipientName"`
	Status          string    `dynamodbav:"Status" json:"status"` // "pending", "accepted", "rejected"
	CreatedAt       time.Time `dynamodbav:"CreatedAt" json:"createdAt"`
}

// SendFriendRequestInput payload for POST /friends/request.
type SendFriendRequestInput struct {
	Target string `json:"target"` // User handle (e.g. @bob) or email
}

// RespondFriendRequestInput payload for POST /friends/request/respond.
type RespondFriendRequestInput struct {
	RequestID string `json:"requestId"`
	Action    string `json:"action"` // "accept" or "reject"
}

// Community represents an accountability group.
type Community struct {
	ID          string   `dynamodbav:"ID" json:"id"`
	Name        string   `dynamodbav:"Name" json:"name"`
	Description string   `dynamodbav:"Description" json:"description"`
	Category    string   `dynamodbav:"Category" json:"category"`
	Members     int      `dynamodbav:"Members" json:"members"`
	Streak      string   `dynamodbav:"Streak" json:"streak"`
	IsJoined    bool     `dynamodbav:"-" json:"isJoined"`
	Tags        []string `dynamodbav:"Tags,omitempty" json:"tags,omitempty"`
}

// JoinCommunityInput payload for POST /communities/join.
type JoinCommunityInput struct {
	CommunityID string `json:"communityId"`
}
