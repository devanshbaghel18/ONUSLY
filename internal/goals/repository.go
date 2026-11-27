package goals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Repository struct {
	db        *dynamodb.Client
	tableName string
	mu        sync.RWMutex
	memGoals  map[string]Goal
}

const localGoalsFallbackPath = "local_goals_fallback.json"

func (r *Repository) loadLocalFallback() {
	data, err := os.ReadFile(localGoalsFallbackPath)
	if err != nil {
		return
	}
	var loaded map[string]Goal
	if err := json.Unmarshal(data, &loaded); err == nil && len(loaded) > 0 {
		r.mu.Lock()
		r.memGoals = loaded
		r.mu.Unlock()
	}
}

func (r *Repository) saveLocalFallback() {
	if r.memGoals == nil {
		return
	}
	data, err := json.MarshalIndent(r.memGoals, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(localGoalsFallbackPath, data, 0644)
}

func NewRepository(db *dynamodb.Client, tableName string) *Repository {
	repo := &Repository{
		db:        db,
		tableName: tableName,
		memGoals:  make(map[string]Goal),
	}
	if db == nil {
		repo.loadLocalFallback()
	}
	return repo
}

func (r *Repository) Create(ctx context.Context, goal Goal) error {
	if goal.PK == "" && goal.OwnerID != "" {
		goal.PK = "USER#" + goal.OwnerID
	}
	if goal.SK == "" && goal.ID != "" {
		goal.SK = "GOAL#" + goal.ID
	}
	if goal.Version == 0 {
		goal.Version = 1
	}
	if goal.Status == "" {
		goal.Status = "active"
	}
	goal.GSI1PK = "GOAL#" + goal.ID
	goal.GSI1SK = "USER#" + goal.OwnerID

	if goal.ApproverID != "" {
		goal.GSI2PK = FormatApproverGSI2PK(goal.ApproverID)
		goal.GSI2SK = FormatApproverGSI2SK(goal.Status, goal.ID)
	} else if goal.ApproverEmail != "" {
		goal.GSI2PK = FormatApproverGSI2PK(goal.ApproverEmail)
		goal.GSI2SK = FormatApproverGSI2SK(goal.Status, goal.ID)
	}

	if r.db == nil {
		r.mu.Lock()
		if r.memGoals == nil {
			r.memGoals = make(map[string]Goal)
		}
		r.memGoals[goal.OwnerID+"#"+goal.ID] = goal
		r.saveLocalFallback()
		r.mu.Unlock()
		return nil
	}

	item, err := attributevalue.MarshalMap(goal)
	if err != nil {
		return err
	}

	putCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err = r.db.PutItem(putCtx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) GetByID(ctx context.Context, ownerID, goalID string) (*Goal, error) {
	if r.db == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if g, ok := r.memGoals[ownerID+"#"+goalID]; ok {
			cp := g
			return &cp, nil
		}
		return nil, fmt.Errorf("goal not found")
	}

	getCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := r.db.GetItem(getCtx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
			"SK": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
		},
	})

	if err != nil {
		return nil, err
	}

	if len(result.Item) == 0 {
		return nil, fmt.Errorf("goal not found")
	}

	var goal Goal

	if err := attributevalue.UnmarshalMap(result.Item, &goal); err != nil {
		return nil, err
	}

	return &goal, nil
}

func (r *Repository) FindByID(ctx context.Context, goalID string) (*Goal, error) {
	if r.db == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		for _, g := range r.memGoals {
			if g.ID == goalID {
				cp := g
				return &cp, nil
			}
		}
		return nil, fmt.Errorf("goal not found")
	}

	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(out.Items) > 0 {
		var goal Goal
		if err := attributevalue.UnmarshalMap(out.Items[0], &goal); err != nil {
			return nil, err
		}
		return &goal, nil
	}

	// Only run table Scan fallback when explicitly opted in for pre-GSI1 legacy records.
	if os.Getenv("ENABLE_LEGACY_GOAL_SCAN") == "true" {
		var startKey map[string]types.AttributeValue
		for {
			scanInput := &dynamodb.ScanInput{
				TableName:        aws.String(r.tableName),
				FilterExpression: aws.String("ID = :id AND begins_with(SK, :sk)"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":id": &types.AttributeValueMemberS{Value: goalID},
					":sk": &types.AttributeValueMemberS{Value: "GOAL#"},
				},
				ExclusiveStartKey: startKey,
			}

			result, err := r.db.Scan(ctx, scanInput)
			if err != nil {
				return nil, err
			}

			if len(result.Items) > 0 {
				var goal Goal
				if err := attributevalue.UnmarshalMap(result.Items[0], &goal); err != nil {
					return nil, err
				}
				return &goal, nil
			}

			if len(result.LastEvaluatedKey) == 0 {
				break
			}
			startKey = result.LastEvaluatedKey
		}
	}

	return nil, fmt.Errorf("goal not found")
}

func (r *Repository) List(ctx context.Context, ownerID string) ([]Goal, error) {
	if r.db == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []Goal
		for _, g := range r.memGoals {
			if g.OwnerID == ownerID {
				res = append(res, g)
			}
		}
		if res == nil {
			res = []Goal{}
		}
		return res, nil
	}

	listCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := r.db.Query(listCtx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{
				Value: "USER#" + ownerID,
			},
			":sk": &types.AttributeValueMemberS{
				Value: "GOAL#",
			},
		},
	})

	if err != nil {
		return nil, err
	}

	var goals []Goal

	if err := attributevalue.UnmarshalListOfMaps(result.Items, &goals); err != nil {
		return nil, err
	}

	return goals, nil
}

func (r *Repository) ListForApprover(ctx context.Context, approverID, approverEmail string) ([]Goal, error) {
	return r.ListByApprover(ctx, approverID, approverEmail, "", "")
}

// ListByApprover queries goals where caller is the assigned approver, optionally filtering by status.
// It queries the GSI2 index by approver ID, email, and handle, falling back to in-memory/table scan.
func (r *Repository) ListByApprover(
	ctx context.Context,
	approverID string,
	approverEmail string,
	approverHandle string,
	statusFilter string,
) ([]Goal, error) {
	approverID = strings.TrimSpace(approverID)
	approverEmail = strings.ToLower(strings.TrimSpace(approverEmail))
	cleanHandle := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(approverHandle), "@"))
	statusFilter = strings.ToLower(strings.TrimSpace(statusFilter))

	// Map human filter to canonical goal status:
	// "pending"  -> "proof_submitted"
	// "approved" -> "completed"
	// "rejected" -> "rejected"
	// "active"   -> "active"
	var targetStatus string
	switch statusFilter {
	case "pending":
		targetStatus = "proof_submitted"
	case "approved":
		targetStatus = "completed"
	case "rejected":
		targetStatus = "rejected"
	case "active":
		targetStatus = "active"
	case "all", "":
		targetStatus = ""
	default:
		targetStatus = statusFilter
	}

	var goals []Goal
	seen := make(map[string]bool)

	// Helper to run GSI2 query for a specific partition key
	queryGSI2 := func(pkValue string) []Goal {
		if r.db == nil || pkValue == "" {
			return nil
		}
		listCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
		defer cancel()

		var keyCond string
		vals := map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pkValue},
		}

		if targetStatus != "" {
			keyCond = "GSI2PK = :pk AND begins_with(GSI2SK, :sk)"
			vals[":sk"] = &types.AttributeValueMemberS{Value: "STATUS#" + targetStatus + "#"}
		} else {
			keyCond = "GSI2PK = :pk"
		}

		result, err := r.db.Query(listCtx, &dynamodb.QueryInput{
			TableName:                 aws.String(r.tableName),
			IndexName:                 aws.String("GSI2"),
			KeyConditionExpression:    aws.String(keyCond),
			ExpressionAttributeValues: vals,
		})
		if err != nil {
			return nil
		}
		var matched []Goal
		_ = attributevalue.UnmarshalListOfMaps(result.Items, &matched)
		return matched
	}

	// 1. Collect candidate partition keys to query on GSI2
	var pks []string
	if approverID != "" {
		pks = append(pks, FormatApproverGSI2PK(approverID))
		pks = append(pks, FormatApproverGSI2PK("partner-"+approverID))
	}
	if approverEmail != "" {
		pks = append(pks, FormatApproverGSI2PK(approverEmail))
	}
	if cleanHandle != "" {
		pks = append(pks, FormatApproverGSI2PK(cleanHandle))
		pks = append(pks, FormatApproverGSI2PK("partner-"+cleanHandle))
	}

	for _, pk := range pks {
		for _, g := range queryGSI2(pk) {
			if !seen[g.ID] {
				// Invariant: Goal owner cannot approve their own goal
				if approverID != "" && g.OwnerID == approverID {
					continue
				}
				if targetStatus == "" || g.Status == targetStatus {
					seen[g.ID] = true
					goals = append(goals, g)
				}
			}
		}
	}

	// 2. Merge/fallback with local memory cache (essential for tests and offline/local fallback)
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, g := range r.memGoals {
		if seen[g.ID] {
			continue
		}
		// Invariant: Goal owner cannot approve their own goal
		if approverID != "" && g.OwnerID == approverID {
			continue
		}

		matches := false
		if approverID != "" && (g.ApproverID == approverID || g.ApproverID == "partner-"+approverID) {
			matches = true
		}
		if approverEmail != "" && strings.EqualFold(strings.TrimSpace(g.ApproverEmail), approverEmail) {
			matches = true
		}
		if cleanHandle != "" {
			if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(g.ApproverEmail), "@"), cleanHandle) ||
				strings.EqualFold(g.ApproverID, "partner-"+cleanHandle) ||
				strings.EqualFold(g.ApproverID, cleanHandle) {
				matches = true
			}
		}

		if matches {
			if targetStatus == "" || g.Status == targetStatus {
				seen[g.ID] = true
				goals = append(goals, g)
			}
		}
	}

	if goals == nil {
		goals = []Goal{}
	}

	return goals, nil
}

func (r *Repository) Delete(ctx context.Context, ownerID, goalID string) error {
	if r.db == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.memGoals == nil {
			return fmt.Errorf("goal not found")
		}
		key := ownerID + "#" + goalID
		g, ok := r.memGoals[key]
		if !ok {
			return fmt.Errorf("goal not found")
		}
		if g.Status == "proof_submitted" {
			return ErrGoalInReview
		}
		if g.Status != "completed" && (len(g.BlockedApps) > 0 || len(g.BlockedDomains) > 0 || len(g.ApplicationTargets) > 0) {
			return ErrGoalLocked
		}
		delete(r.memGoals, key)
		r.saveLocalFallback()
		return nil
	}

	deleteCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := r.db.DeleteItem(deleteCtx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
			"SK": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
		},
		ConditionExpression: aws.String(
			"attribute_not_exists(PK) OR (" +
				"#status <> :submittedStatus AND (" +
				"#status = :completedStatus OR " +
				"((attribute_not_exists(#blockedApps) OR size(#blockedApps) = :zero) AND " +
				"(attribute_not_exists(#blockedDomains) OR size(#blockedDomains) = :zero))))",
		),
		ExpressionAttributeNames: map[string]string{
			"#status":         "Status",
			"#blockedApps":    "BlockedApps",
			"#blockedDomains": "BlockedDomains",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":completedStatus": &types.AttributeValueMemberS{Value: "completed"},
			":submittedStatus": &types.AttributeValueMemberS{Value: "proof_submitted"},
			":zero":            &types.AttributeValueMemberN{Value: "0"},
		},
	})
	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		if errors.As(err, &condFailed) || strings.Contains(err.Error(), "ConditionalCheckFailed") {
			return ErrGoalLocked
		}
		return err
	}

	r.mu.Lock()
	if r.memGoals != nil {
		delete(r.memGoals, ownerID+"#"+goalID)
	}
	r.mu.Unlock()

	return nil
}

func (r *Repository) Update(
	ctx context.Context,
	ownerID string,
	goalID string,
	input UpdateGoalInput,
) (*Goal, error) {

	var setParts []string
	names := map[string]string{}
	values := map[string]types.AttributeValue{}

	updatedAt := input.UpdatedAt
	if updatedAt == "" {
		updatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	setParts = append(setParts, "#updatedAt = :updatedAt")
	names["#updatedAt"] = "UpdatedAt"
	values[":updatedAt"] = &types.AttributeValueMemberS{Value: updatedAt}

	if input.Title != nil {
		setParts = append(setParts, "#title = :title")
		names["#title"] = "Title"
		values[":title"] = &types.AttributeValueMemberS{Value: *input.Title}
	}

	if input.Description != nil {
		setParts = append(setParts, "#description = :description")
		names["#description"] = "Description"
		values[":description"] = &types.AttributeValueMemberS{Value: *input.Description}
	}

	if input.ApprovalType != nil {
		setParts = append(setParts, "#approvalType = :approvalType")
		names["#approvalType"] = "ApprovalType"
		values[":approvalType"] = &types.AttributeValueMemberS{Value: *input.ApprovalType}
	}

	if input.ApproverEmail != nil {
		setParts = append(setParts, "#approverEmail = :approverEmail")
		names["#approverEmail"] = "ApproverEmail"
		values[":approverEmail"] = &types.AttributeValueMemberS{Value: *input.ApproverEmail}
	}

	if input.ApproverID != nil {
		setParts = append(setParts, "#approverId = :approverId")
		names["#approverId"] = "ApproverID"
		values[":approverId"] = &types.AttributeValueMemberS{Value: *input.ApproverID}
	}

	var removeParts []string

	if input.ApproverID != nil && *input.ApproverID != "" {
		setParts = append(setParts, "#gsi2pk = :gsi2pk")
		names["#gsi2pk"] = "GSI2PK"
		values[":gsi2pk"] = &types.AttributeValueMemberS{Value: FormatApproverGSI2PK(*input.ApproverID)}
	} else if input.ApproverEmail != nil && *input.ApproverEmail != "" {
		setParts = append(setParts, "#gsi2pk = :gsi2pk")
		names["#gsi2pk"] = "GSI2PK"
		values[":gsi2pk"] = &types.AttributeValueMemberS{Value: FormatApproverGSI2PK(*input.ApproverEmail)}
	} else if input.ApproverID != nil && *input.ApproverID == "" {
		names["#gsi2pk"] = "GSI2PK"
		names["#gsi2sk"] = "GSI2SK"
		removeParts = append(removeParts, "#gsi2pk", "#gsi2sk")
	}

	if input.BlockedApps != nil {
		setParts = append(setParts, "#blockedApps = :blockedApps")
		names["#blockedApps"] = "BlockedApps"
		avApps, err := attributevalue.Marshal(*input.BlockedApps)
		if err != nil {
			return nil, err
		}
		values[":blockedApps"] = avApps
	}

	if input.BlockedDomains != nil {
		setParts = append(setParts, "#blockedDomains = :blockedDomains")
		names["#blockedDomains"] = "BlockedDomains"
		avDomains, err := attributevalue.Marshal(*input.BlockedDomains)
		if err != nil {
			return nil, err
		}
		values[":blockedDomains"] = avDomains
	}

	if input.ApplicationTargets != nil {
		setParts = append(setParts, "#applicationTargets = :applicationTargets")
		names["#applicationTargets"] = "ApplicationTargets"
		avTargets, err := attributevalue.Marshal(*input.ApplicationTargets)
		if err != nil {
			return nil, err
		}
		values[":applicationTargets"] = avTargets
	}

	conditionParts := []string{"attribute_exists(PK)"}

	if input.DisallowNoneIfLocked {
		conditionParts = append(conditionParts, "(#status = :completedStatus OR ((attribute_not_exists(#blockedApps) OR size(#blockedApps) = :zero) AND (attribute_not_exists(#blockedDomains) OR size(#blockedDomains) = :zero)))")
		names["#status"] = "Status"
		names["#blockedApps"] = "BlockedApps"
		names["#blockedDomains"] = "BlockedDomains"
		values[":completedStatus"] = &types.AttributeValueMemberS{Value: "completed"}
		values[":zero"] = &types.AttributeValueMemberN{Value: "0"}
	}

	if input.CheckTargetsUnchanged {
		if len(input.ExpectedBlockedApps) > 0 {
			conditionParts = append(conditionParts, "#blockedApps = :expectedBlockedApps")
			names["#blockedApps"] = "BlockedApps"
			expApps, err := attributevalue.Marshal(input.ExpectedBlockedApps)
			if err != nil {
				return nil, err
			}
			values[":expectedBlockedApps"] = expApps
		} else {
			conditionParts = append(conditionParts, "(attribute_not_exists(#blockedApps) OR size(#blockedApps) = :zero)")
			names["#blockedApps"] = "BlockedApps"
			values[":zero"] = &types.AttributeValueMemberN{Value: "0"}
		}

		if len(input.ExpectedBlockedDomains) > 0 {
			conditionParts = append(conditionParts, "#blockedDomains = :expectedBlockedDomains")
			names["#blockedDomains"] = "BlockedDomains"
			expDomains, err := attributevalue.Marshal(input.ExpectedBlockedDomains)
			if err != nil {
				return nil, err
			}
			values[":expectedBlockedDomains"] = expDomains
		} else {
			conditionParts = append(conditionParts, "(attribute_not_exists(#blockedDomains) OR size(#blockedDomains) = :zero)")
			names["#blockedDomains"] = "BlockedDomains"
			values[":zero"] = &types.AttributeValueMemberN{Value: "0"}
		}
	}

	if input.RequireFriendAccountability {
		conditionParts = append(conditionParts, "#approvalType = :friendType")
		names["#approvalType"] = "ApprovalType"
		values[":friendType"] = &types.AttributeValueMemberS{Value: "friend"}
	}

	// Optimistic concurrency control: verify expected version or updatedAt before applying mutation
	setParts = append(setParts, "#version = if_not_exists(#version, :zero) + :one")
	names["#version"] = "Version"
	values[":zero"] = &types.AttributeValueMemberN{Value: "0"}
	values[":one"] = &types.AttributeValueMemberN{Value: "1"}

	if input.ExpectedVersion != nil {
		conditionParts = append(conditionParts, "#version = :expectedVersion")
		names["#version"] = "Version"
		values[":expectedVersion"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", *input.ExpectedVersion)}
	} else if input.ExpectedUpdatedAt != nil && *input.ExpectedUpdatedAt != "" {
		conditionParts = append(conditionParts, "#updatedAt = :expectedUpdatedAt")
		names["#updatedAt"] = "UpdatedAt"
		values[":expectedUpdatedAt"] = &types.AttributeValueMemberS{Value: *input.ExpectedUpdatedAt}
	}

	updateExpression := "SET " + strings.Join(setParts, ", ")
	if len(removeParts) > 0 {
		updateExpression += " REMOVE " + strings.Join(removeParts, ", ")
	}
	conditionExpression := strings.Join(conditionParts, " AND ")

	if r.db == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.memGoals != nil {
			key := ownerID + "#" + goalID
			if g, ok := r.memGoals[key]; ok {
				hasLockedTargets := len(g.BlockedApps) > 0 || len(g.BlockedDomains) > 0 || len(g.ApplicationTargets) > 0
				if input.DisallowNoneIfLocked && g.Status != "completed" && hasLockedTargets {
					return nil, ErrAccountabilityRequired
				}
				if input.RequireFriendAccountability && g.Status != "completed" && g.ApprovalType != "friend" {
					return nil, ErrAccountabilityRequired
				}
				if input.ExpectedVersion != nil && g.Version != *input.ExpectedVersion {
					return nil, ErrGoalConflict
				}
				if input.ExpectedUpdatedAt != nil && *input.ExpectedUpdatedAt != "" && g.UpdatedAt != *input.ExpectedUpdatedAt {
					return nil, ErrGoalConflict
				}
				if input.CheckTargetsUnchanged {
					if len(g.BlockedApps) != len(input.ExpectedBlockedApps) || len(g.BlockedDomains) != len(input.ExpectedBlockedDomains) {
						return nil, ErrTargetsLocked
					}
				}
				if input.Title != nil {
					g.Title = *input.Title
				}
				if input.Description != nil {
					g.Description = *input.Description
				}
				if input.ApprovalType != nil {
					g.ApprovalType = *input.ApprovalType
				}
				if input.ApproverEmail != nil {
					g.ApproverEmail = *input.ApproverEmail
					if g.ApproverID == "" && *input.ApproverEmail != "" {
						g.GSI2PK = FormatApproverGSI2PK(*input.ApproverEmail)
						g.GSI2SK = FormatApproverGSI2SK(g.Status, g.ID)
					}
				}
				if input.ApproverID != nil {
					g.ApproverID = *input.ApproverID
					if *input.ApproverID != "" {
						g.GSI2PK = FormatApproverGSI2PK(*input.ApproverID)
						g.GSI2SK = FormatApproverGSI2SK(g.Status, g.ID)
					} else if input.ApproverEmail == nil || *input.ApproverEmail == "" {
						g.GSI2PK = ""
						g.GSI2SK = ""
					}
				}
				if input.BlockedApps != nil {
					g.BlockedApps = *input.BlockedApps
				}
				if input.BlockedDomains != nil {
					g.BlockedDomains = *input.BlockedDomains
				}
				if input.ApplicationTargets != nil {
					g.ApplicationTargets = *input.ApplicationTargets
				}
				g.UpdatedAt = input.UpdatedAt
				g.Version++
				r.memGoals[key] = g
				r.saveLocalFallback()
				cp := g
				return &cp, nil
			}
		}
		return nil, fmt.Errorf("goal not found")
	}

	result, err := r.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{
				Value: "USER#" + ownerID,
			},
			"SK": &types.AttributeValueMemberS{
				Value: "GOAL#" + goalID,
			},
		},
		UpdateExpression:          aws.String(updateExpression),
		ConditionExpression:       aws.String(conditionExpression),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
		ReturnValues:              types.ReturnValueAllNew,
	})

	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		if errors.As(err, &condFailed) || strings.Contains(err.Error(), "ConditionalCheckFailed") {
			if input.ExpectedVersion != nil || (input.ExpectedUpdatedAt != nil && *input.ExpectedUpdatedAt != "") {
				return nil, ErrGoalConflict
			}
			if input.DisallowNoneIfLocked || input.RequireFriendAccountability {
				return nil, ErrAccountabilityRequired
			}
			if input.CheckTargetsUnchanged {
				return nil, ErrTargetsLocked
			}
			return nil, ErrGoalLocked
		}
		return nil, err
	}

	var goal Goal

	if err := attributevalue.UnmarshalMap(result.Attributes, &goal); err != nil {
		return nil, err
	}

	return &goal, nil
}

func (r *Repository) UpdateStatus(
	ctx context.Context,
	ownerID string,
	goalID string,
	status string,
) (*Goal, error) {
	gsi2sk := FormatApproverGSI2SK(status, goalID)

	if r.db == nil {
		r.mu.Lock()
		var inMemGoal *Goal
		if r.memGoals != nil {
			key := ownerID + "#" + goalID
			if g, ok := r.memGoals[key]; ok {
				g.Status = status
				if g.ApproverID != "" || g.ApproverEmail != "" {
					g.GSI2SK = gsi2sk
				}
				r.memGoals[key] = g
				r.saveLocalFallback()
				cp := g
				inMemGoal = &cp
			}
		}
		r.mu.Unlock()
		if inMemGoal != nil {
			return inMemGoal, nil
		}
		return nil, fmt.Errorf("goal not found")
	}

	updateCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := r.db.UpdateItem(updateCtx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{
				Value: "USER#" + ownerID,
			},
			"SK": &types.AttributeValueMemberS{
				Value: "GOAL#" + goalID,
			},
		},
		UpdateExpression: aws.String(
			"SET #status = :status, #gsi2sk = :gsi2sk",
		),
		ExpressionAttributeNames: map[string]string{
			"#status": "Status",
			"#gsi2sk": "GSI2SK",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{
				Value: status,
			},
			":gsi2sk": &types.AttributeValueMemberS{
				Value: gsi2sk,
			},
		},
		ReturnValues: types.ReturnValueAllNew,
	})

	if err != nil {
		return nil, err
	}

	var goal Goal

	if err := attributevalue.UnmarshalMap(result.Attributes, &goal); err != nil {
		return nil, err
	}

	return &goal, nil
}
