package friends

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
	"github.com/google/uuid"
)

type UserResolver interface {
	ResolveUser(ctx context.Context, identifier string) (userID, email, handle string, err error)
}

type MessageBroadcaster interface {
	SendToUser(userID string, event realtime.Event) error
}

type Service struct {
	repo         *Repository
	userResolver UserResolver
	broadcaster  MessageBroadcaster
}

func NewService(repo *Repository, resolver UserResolver, broadcaster MessageBroadcaster) *Service {
	return &Service{
		repo:         repo,
		userResolver: resolver,
		broadcaster:  broadcaster,
	}
}

func (s *Service) SetBroadcaster(b MessageBroadcaster) {
	s.broadcaster = b
}

func (s *Service) SetUserResolver(r UserResolver) {
	s.userResolver = r
}

// SendRequest sends a friend request to a target user resolved by handle or email.
func (s *Service) SendRequest(ctx context.Context, callerID, callerEmail, callerHandle string, target string) (*FriendRequest, error) {
	cleanTarget := strings.TrimSpace(target)
	if cleanTarget == "" {
		return nil, errors.New("target handle or email is required")
	}

	if s.userResolver == nil {
		return nil, errors.New("user resolver not configured")
	}

	targetID, targetEmail, targetHandle, err := s.userResolver.ResolveUser(ctx, cleanTarget)
	if err != nil || targetID == "" {
		return nil, fmt.Errorf("user %q not found", cleanTarget)
	}

	if targetID == callerID {
		return nil, errors.New("cannot send friend request to yourself")
	}

	// Check if already friends
	existingFriends, err := s.repo.GetFriends(ctx, callerID)
	if err == nil {
		for _, f := range existingFriends {
			if f.FriendID == targetID {
				return nil, errors.New("already friends with this user")
			}
		}
	}

	// Check if outgoing request already pending
	outgoing, err := s.repo.GetOutgoingRequests(ctx, callerID)
	if err == nil {
		for _, req := range outgoing {
			if req.RecipientID == targetID && req.Status == "pending" {
				return nil, errors.New("friend request already pending")
			}
		}
	}

	callerName := callerHandle
	if callerName == "" {
		callerName = strings.Split(callerEmail, "@")[0]
	}

	targetName := targetHandle
	if targetName == "" {
		targetName = strings.Split(targetEmail, "@")[0]
	}

	req := FriendRequest{
		ID:              uuid.New().String(),
		SenderID:        callerID,
		SenderHandle:    callerHandle,
		SenderEmail:     callerEmail,
		SenderName:      callerName,
		RecipientID:     targetID,
		RecipientHandle: targetHandle,
		RecipientEmail:  targetEmail,
		RecipientName:   targetName,
		Status:          "pending",
		CreatedAt:       time.Now().UTC(),
	}

	if err := s.repo.SaveFriendRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("save friend request: %w", err)
	}

	// Broadcast realtime event to recipient if broadcaster is present
	if s.broadcaster != nil {
		_ = s.broadcaster.SendToUser(targetID, realtime.Event{
			Type: "friend.request",
			Payload: map[string]interface{}{
				"request": req,
			},
		})
	}

	return &req, nil
}

// GetIncomingRequests retrieves pending incoming friend requests for a user.
func (s *Service) GetIncomingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	return s.repo.GetIncomingRequests(ctx, userID)
}

// GetOutgoingRequests retrieves pending outgoing friend requests for a user.
func (s *Service) GetOutgoingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	return s.repo.GetOutgoingRequests(ctx, userID)
}

// RespondRequest accepts or rejects an incoming friend request.
func (s *Service) RespondRequest(ctx context.Context, callerID string, reqID string, action string) error {
	cleanAction := strings.ToLower(strings.TrimSpace(action))
	if cleanAction != "accept" && cleanAction != "reject" {
		return errors.New("action must be 'accept' or 'reject'")
	}

	req, err := s.repo.GetIncomingRequestByID(ctx, callerID, reqID)
	if err != nil {
		return fmt.Errorf("get incoming request: %w", err)
	}
	if req == nil {
		return errors.New("friend request not found or unauthorized")
	}

	if cleanAction == "accept" {
		if err := s.repo.AcceptFriendRequest(ctx, *req); err != nil {
			return fmt.Errorf("accept friend request: %w", err)
		}

		// Broadcast acceptance to both users
		if s.broadcaster != nil {
			event := realtime.Event{
				Type: "friend.accepted",
				Payload: map[string]interface{}{
					"friendId":   req.RecipientID,
					"senderId":   req.SenderID,
					"friendName": req.RecipientName,
					"senderName": req.SenderName,
				},
			}
			_ = s.broadcaster.SendToUser(req.SenderID, event)
			_ = s.broadcaster.SendToUser(req.RecipientID, event)
		}
	} else {
		if err := s.repo.RejectFriendRequest(ctx, *req); err != nil {
			return fmt.Errorf("reject friend request: %w", err)
		}
	}

	return nil
}

// GetFriends returns all accepted friends of the specified user.
func (s *Service) GetFriends(ctx context.Context, userID string) ([]Friend, error) {
	return s.repo.GetFriends(ctx, userID)
}

// DeleteFriend removes a friendship.
func (s *Service) DeleteFriend(ctx context.Context, userID, friendID string) error {
	return s.repo.DeleteFriend(ctx, userID, friendID)
}

// DefaultTrendingCommunities returns curated popular communities for accountability.
var DefaultTrendingCommunities = []Community{
	{
		ID:          "comm-early-risers",
		Name:        "Early Risers 5AM Club",
		Category:    "Mindset & Habits",
		Members:     428,
		Streak:      "14 Days",
		Description: "Wake up at 5:00 AM, submit daily morning proof, and win your day before 8 AM.",
		Tags:        []string{"Habits", "Morning", "Discipline"},
	},
	{
		ID:          "comm-leetcode",
		Name:        "LeetCode & Algorithms Sprint",
		Category:    "Coding & Tech",
		Members:     615,
		Streak:      "21 Days",
		Description: "Solve at least 1 Medium problem every single day. Code submissions & acceptance screenshots required.",
		Tags:        []string{"Coding", "Algorithms", "Career"},
	},
	{
		ID:          "comm-fitness",
		Name:        "Iron Will Gym & Fitness",
		Category:    "Health & Fitness",
		Members:     890,
		Streak:      "30 Days",
		Description: "Consistent physical training: gym check-in, lifting log, or 10,000 steps proof.",
		Tags:        []string{"Gym", "Fitness", "Health"},
	},
	{
		ID:          "comm-deep-work",
		Name:        "Deep Work 4-Hour Club",
		Category:    "Productivity",
		Members:     340,
		Streak:      "7 Days",
		Description: "4 hours of distraction-free focused work with site/app blockers engaged.",
		Tags:        []string{"Focus", "Productivity", "Flow"},
	},
	{
		ID:          "comm-book-reading",
		Name:        "Daily 20-Page Readers",
		Category:    "Learning",
		Members:     210,
		Streak:      "18 Days",
		Description: "Read at least 20 pages of non-fiction / technical literature daily.",
		Tags:        []string{"Reading", "Knowledge", "Books"},
	},
}

// GetTrendingCommunities returns all trending communities with the user's joined status.
func (s *Service) GetTrendingCommunities(ctx context.Context, userID string) ([]Community, error) {
	joinedIDs, err := s.repo.GetJoinedCommunities(ctx, userID)
	if err != nil {
		joinedIDs = nil
	}

	joinedMap := make(map[string]bool)
	for _, id := range joinedIDs {
		joinedMap[id] = true
	}

	result := make([]Community, len(DefaultTrendingCommunities))
	for i, c := range DefaultTrendingCommunities {
		cCopy := c
		cCopy.IsJoined = joinedMap[c.ID]
		result[i] = cCopy
	}

	return result, nil
}

// JoinCommunity marks a user as joined for a community.
func (s *Service) JoinCommunity(ctx context.Context, userID, communityID string) error {
	cleanID := strings.TrimSpace(communityID)
	if cleanID == "" {
		return errors.New("community ID is required")
	}
	return s.repo.JoinCommunity(ctx, userID, cleanID)
}

// LeaveCommunity removes a user from a community.
func (s *Service) LeaveCommunity(ctx context.Context, userID, communityID string) error {
	cleanID := strings.TrimSpace(communityID)
	if cleanID == "" {
		return errors.New("community ID is required")
	}
	return s.repo.LeaveCommunity(ctx, userID, cleanID)
}

// GetJoinedCommunities returns the list of joined community objects for a user.
func (s *Service) GetJoinedCommunities(ctx context.Context, userID string) ([]Community, error) {
	all, err := s.GetTrendingCommunities(ctx, userID)
	if err != nil {
		return nil, err
	}
	joined := make([]Community, 0)
	for _, c := range all {
		if c.IsJoined {
			joined = append(joined, c)
		}
	}
	return joined, nil
}
