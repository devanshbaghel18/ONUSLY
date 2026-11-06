package enforcement

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

type GoalLister interface {
	List(ctx context.Context, ownerID string) ([]goals.Goal, error)
}

type GoalGetter interface {
	GetByID(ctx context.Context, ownerID, goalID string) (*goals.Goal, error)
}

type NotificationSender interface {
	CreateFromEnforcementLapsed(ctx context.Context, recipientID, episodeID, goalID, title, message string) error
}

type TokenGenerator interface {
	GenerateToken(userID, email, handle, name string) (string, error)
}

type Service struct {
	goalLister         GoalLister
	goalGetter         GoalGetter
	deviceRepo         DeviceRepository
	notificationSender NotificationSender
	tokenGenerator     TokenGenerator
}

func NewService(goalLister GoalLister) *Service {
	s := &Service{
		goalLister: goalLister,
	}
	if getter, ok := goalLister.(GoalGetter); ok {
		s.goalGetter = getter
	}
	return s
}

func (s *Service) SetGoalGetter(getter GoalGetter) {
	s.goalGetter = getter
}

func (s *Service) SetDeviceRepository(repo DeviceRepository) {
	s.deviceRepo = repo
}

func (s *Service) SetNotificationSender(sender NotificationSender) {
	s.notificationSender = sender
}

func (s *Service) SetTokenGenerator(gen TokenGenerator) {
	s.tokenGenerator = gen
}

// canonicalTarget has fixed JSON field order for deterministic canonical JSON serialization.
type canonicalTarget struct {
	GoalID    string   `json:"goalId"`
	GoalTitle string   `json:"goalTitle"`
	Status    string   `json:"status"`
	Apps      []string `json:"apps"`
	Domains   []string `json:"domains"`
}

// ComputeVersion computes v1- + first 16 hex characters of SHA-256 over the canonical JSON of targets.
func ComputeVersion(targets []TargetItem) string {
	list := make([]canonicalTarget, len(targets))
	for i, t := range targets {
		apps := t.Apps
		if apps == nil {
			apps = []string{}
		}
		domains := t.Domains
		if domains == nil {
			domains = []string{}
		}
		list[i] = canonicalTarget{
			GoalID:    t.GoalID,
			GoalTitle: t.GoalTitle,
			Status:    t.Status,
			Apps:      apps,
			Domains:   domains,
		}
	}

	data, _ := json.Marshal(list)
	h := sha256.Sum256(data)
	hexHash := hex.EncodeToString(h[:])
	return "v1-" + hexHash[:16]
}

func (s *Service) GetBlocklist(ctx context.Context, userID string) (*BlocklistResponse, error) {
	goalsList, err := s.goalLister.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	var targets []TargetItem
	for _, g := range goalsList {
		// Include goals only when: status == active OR proof_submitted AND at least one target exists
		if g.Status != "active" && g.Status != "proof_submitted" {
			continue
		}
		if len(g.BlockedApps) == 0 && len(g.BlockedDomains) == 0 {
			continue
		}

		apps := append([]string(nil), g.BlockedApps...)
		domains := append([]string(nil), g.BlockedDomains...)

		expandedApps, expandedDomains, err := goals.ExpandAssociatedTargets(apps, domains)
		if err == nil {
			apps = expandedApps
			domains = expandedDomains
		} else {
			sort.Strings(apps)
			sort.Strings(domains)
		}
		if apps == nil {
			apps = []string{}
		}
		if domains == nil {
			domains = []string{}
		}

		targets = append(targets, TargetItem{
			GoalID:    g.ID,
			GoalTitle: g.Title,
			Status:    g.Status,
			Apps:      apps,
			Domains:   domains,
			createdAt: g.CreatedAt,
		})
	}

	// Ordering: targets by creation time, then goal ID
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].createdAt == targets[j].createdAt {
			return targets[i].GoalID < targets[j].GoalID
		}
		return targets[i].createdAt < targets[j].createdAt
	})

	version := ComputeVersion(targets)
	now := time.Now().UTC().Format(time.RFC3339)

	return &BlocklistResponse{
		UserID:      userID,
		Version:     version,
		GeneratedAt: now,
		Targets:     targets,
	}, nil
}

func (s *Service) ProcessHeartbeat(ctx context.Context, userID string, req HeartbeatRequest) (*Device, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if s.deviceRepo == nil {
		return nil, errors.New("device repository not configured")
	}

	existing, _ := s.deviceRepo.GetDevice(ctx, userID, req.DeviceID)

	createdAt := req.SentAt
	lapseEpisodeID := ""
	if existing != nil {
		createdAt = existing.CreatedAt
		lapseEpisodeID = existing.LapseEpisodeID
	}

	isLapsed := !req.Enforcing || len(req.Issues) > 0
	if isLapsed {
		// If not already in an active lapse episode, start a new one and emit notifications
		if lapseEpisodeID == "" {
			lapseEpisodeID = fmt.Sprintf("lapse-%s-%d", req.DeviceID, time.Now().Unix())
			if s.notificationSender != nil {
				// 1. Notify owner
				ownerTitle := "Enforcement Issue Detected"
				issuesStr := strings.Join(req.Issues, ", ")
				if issuesStr == "" {
					issuesStr = "enforcement inactive"
				}
				ownerMsg := fmt.Sprintf("Device (%s/%s) reported enforcement issues: %s", req.ClientType, req.Platform, issuesStr)
				_ = s.notificationSender.CreateFromEnforcementLapsed(ctx, userID, lapseEpisodeID+"-owner", "", ownerTitle, ownerMsg)

				// 2. Notify approvers of active goals
				if s.goalLister != nil {
					userGoals, err := s.goalLister.List(ctx, userID)
					if err == nil {
						for _, g := range userGoals {
							if (g.Status == "active" || g.Status == "proof_submitted") && g.ApproverID != "" && g.ApproverID != "COMMUNITY" {
								approverTitle := "Accountability Alert"
								approverMsg := fmt.Sprintf("An enforcement lapse was detected on one of the devices protecting goal %q.", g.Title)
								_ = s.notificationSender.CreateFromEnforcementLapsed(ctx, g.ApproverID, lapseEpisodeID+"-"+g.ID, g.ID, approverTitle, approverMsg)
							}
						}
					}
				}
			}
		}
	} else {
		// Healthy: clear active lapse episode
		lapseEpisodeID = ""
	}

	now := time.Now().UTC()
	device := Device{
		DeviceID:         req.DeviceID,
		UserID:           userID,
		ClientType:       req.ClientType,
		Platform:         req.Platform,
		Browser:          req.Browser,
		ClientVersion:    req.ClientVersion,
		Enforcing:        req.Enforcing,
		BlocklistVersion: string(req.BlocklistVersion),
		Issues:           req.Issues,
		LastHeartbeatAt:  req.SentAt,
		LapseEpisodeID:   lapseEpisodeID,
		CreatedAt:        createdAt,
		UpdatedAt:        now.Format(time.RFC3339),
		ExpiresAt:        now.Add(30 * 24 * time.Hour).Unix(),
	}

	if err := s.deviceRepo.UpsertDevice(ctx, device); err != nil {
		return nil, err
	}

	return &device, nil
}

func (s *Service) GetDevices(ctx context.Context, userID string) (*DeviceCoverageResponse, error) {
	if s.deviceRepo == nil {
		return nil, errors.New("device repository not configured")
	}

	devices, err := s.deviceRepo.ListDevices(ctx, userID)
	if err != nil {
		return nil, err
	}

	allEnforcing := len(devices) > 0
	for _, d := range devices {
		if !d.Enforcing || len(d.Issues) > 0 {
			allEnforcing = false
			break
		}
	}

	return &DeviceCoverageResponse{
		UserID:     userID,
		TotalCount: len(devices),
		Enforcing:  allEnforcing,
		Devices:    devices,
	}, nil
}

func (s *Service) GetGoalEnforcementStatus(ctx context.Context, callerID, callerEmail, callerHandle, goalID string) (*GoalEnforcementStatusResponse, error) {
	if s.goalGetter == nil || s.deviceRepo == nil {
		return nil, errors.New("dependencies not configured")
	}

	goal, err := s.goalGetter.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	isOwner := goal.OwnerID == callerID
	isApprover := goal.IsApprover(callerID, callerEmail, callerHandle)

	if !isOwner && !isApprover {
		return nil, ErrUnauthorizedGoal
	}

	devices, err := s.deviceRepo.ListDevices(ctx, goal.OwnerID)
	if err != nil {
		return nil, err
	}

	devicesCount := len(devices)
	enforcingCount := 0
	hasIssues := false
	latestSync := ""
	offlineCutoff := time.Now().Add(-24 * time.Hour)

	for _, d := range devices {
		isLapsed := !d.Enforcing || len(d.Issues) > 0
		if hbTime, err := time.Parse(time.RFC3339, d.LastHeartbeatAt); err == nil {
			if hbTime.Before(offlineCutoff) {
				isLapsed = true
			}
		}
		if !isLapsed {
			enforcingCount++
		} else {
			hasIssues = true
		}
		if d.LastHeartbeatAt > latestSync {
			latestSync = d.LastHeartbeatAt
		}
	}

	aggregateStatus := "enforcing"
	if devicesCount == 0 {
		aggregateStatus = "no_devices"
	} else if hasIssues || enforcingCount < devicesCount {
		aggregateStatus = "lapsed"
	}

	resp := &GoalEnforcementStatusResponse{
		GoalID:          goalID,
		AggregateStatus: aggregateStatus,
		DevicesCount:    devicesCount,
		EnforcingCount:  enforcingCount,
		HasIssues:       hasIssues,
		LastSyncAt:      latestSync,
	}

	// Privileged owner view receives concrete device enumeration; approvers receive strictly aggregate
	if isOwner {
		resp.Devices = devices
	}

	return resp, nil
}

const pairingCharset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Excludes easily confused chars (I, O, 0, 1)

func generatePairingCode() (string, error) {
	code := make([]byte, 6)
	charsetLen := big.NewInt(int64(len(pairingCharset)))
	for i := 0; i < 6; i++ {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}
		code[i] = pairingCharset[num.Int64()]
	}
	return string(code), nil
}

func (s *Service) InitiatePairing(ctx context.Context, clientType, platform, deviceID string) (*PairingSession, error) {
	if s.deviceRepo == nil {
		return nil, errors.New("device repository not configured")
	}

	code, err := generatePairingCode()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session := PairingSession{
		Code:       code,
		DeviceID:   deviceID,
		Platform:   platform,
		ClientType: clientType,
		Confirmed:  false,
		CreatedAt:  now.Format(time.RFC3339),
		ExpiresAt:  now.Add(10 * time.Minute).Unix(),
	}

	if err := s.deviceRepo.SavePairingSession(ctx, session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (s *Service) ConfirmPairing(ctx context.Context, code, userID, userEmail, userHandle, userName string) error {
	if s.deviceRepo == nil || s.tokenGenerator == nil {
		return errors.New("dependencies not configured")
	}

	code = strings.ToUpper(strings.TrimSpace(code))
	token, err := s.tokenGenerator.GenerateToken(userID, userEmail, userHandle, userName)
	if err != nil {
		return fmt.Errorf("failed to generate device token: %w", err)
	}

	return s.deviceRepo.ConfirmPairingSession(ctx, code, userID, token)
}

func (s *Service) ExchangePairingToken(ctx context.Context, code string) (string, error) {
	if s.deviceRepo == nil {
		return "", errors.New("device repository not configured")
	}

	code = strings.ToUpper(strings.TrimSpace(code))
	session, err := s.deviceRepo.ConsumePairingSession(ctx, code)
	if err != nil {
		return "", err
	}
	if session.Token == "" {
		return "", errors.New("pairing token not available")
	}
	return session.Token, nil
}
