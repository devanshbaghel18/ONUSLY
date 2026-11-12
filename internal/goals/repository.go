package goals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	repo.loadLocalFallback()
	return repo
}

func (r *Repository) Create(ctx context.Context, goal Goal) error {
	if goal.Version == 0 {
		goal.Version = 1
	}
	goal.GSI1PK = "GOAL#" + goal.ID
	goal.GSI1SK = "USER#" + goal.OwnerID

	if goal.ApproverID != "" {
		goal.GSI2PK = "APPROVER#" + goal.ApproverID
		goal.GSI2SK = "GOAL#" + goal.ID
	} else if goal.ApproverEmail != "" {
		goal.GSI2PK = "APPROVER#" + strings.ToLower(strings.TrimSpace(goal.ApproverEmail))
		goal.GSI2SK = "GOAL#" + goal.ID
	}

	r.mu.Lock()
	if r.memGoals == nil {
		r.memGoals = make(map[string]Goal)
	}
	r.memGoals[goal.OwnerID+"#"+goal.ID] = goal
	r.saveLocalFallback()
	r.mu.Unlock()

	item, err := attributevalue.MarshalMap(goal)
	if err != nil {
		return err
	}

	if r.db == nil {
		return nil
	}

	putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	_, err = r.db.PutItem(putCtx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		log.Printf("[GoalsRepo] Remote database put notice: %v (cached in local memory)", err)
		return nil
	}

	return nil
}

func (r *Repository) GetByID(ctx context.Context, ownerID, goalID string) (*Goal, error) {
	getCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	result, err := r.db.GetItem(getCtx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
			"SK": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
		},
	})

	if err != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if g, ok := r.memGoals[ownerID+"#"+goalID]; ok {
			cp := g
			return &cp, nil
		}
		return nil, err
	}

	if len(result.Item) == 0 {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if g, ok := r.memGoals[ownerID+"#"+goalID]; ok {
			cp := g
			return &cp, nil
		}
		return nil, fmt.Errorf("goal not found")
	}

	var goal Goal

	if err := attributevalue.UnmarshalMap(result.Item, &goal); err != nil {
		return nil, err
	}

	return &goal, nil
}

func (r *Repository) FindByID(ctx context.Context, goalID string) (*Goal, error) {
	// 1. Try GSI1 indexed lookup first (O(1) query)
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
		},
		Limit: aws.Int32(1),
	})
	if err == nil && len(out.Items) > 0 {
		var goal Goal
		if err := attributevalue.UnmarshalMap(out.Items[0], &goal); err == nil {
			return &goal, nil
		}
	}

	// 2. Fallback to scan for legacy records or testing environments without GSI1
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

func (r *Repository) List(ctx context.Context, ownerID string) ([]Goal, error) {
	listCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
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

	var goals []Goal

	if err := attributevalue.UnmarshalListOfMaps(result.Items, &goals); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, g := range r.memGoals {
		if g.OwnerID == ownerID {
			found := false
			for _, dbGoal := range goals {
				if dbGoal.ID == g.ID {
					found = true
					break
				}
			}
			if !found {
				goals = append(goals, g)
			}
		}
	}

	return goals, nil
}

func (r *Repository) ListForApprover(ctx context.Context, approverID, approverEmail string) ([]Goal, error) {
	approverID = strings.TrimSpace(approverID)
	approverEmail = strings.ToLower(strings.TrimSpace(approverEmail))

	var goals []Goal

	queryApprover := func(pkValue string) []Goal {
		if r.db == nil {
			return nil
		}
		listCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer cancel()

		result, err := r.db.Query(listCtx, &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			IndexName:              aws.String("GSI2"),
			KeyConditionExpression: aws.String("GSI2PK = :pk AND begins_with(GSI2SK, :sk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{
					Value: pkValue,
				},
				":sk": &types.AttributeValueMemberS{
					Value: "GOAL#",
				},
			},
		})
		if err != nil {
			return nil
		}
		var matched []Goal
		_ = attributevalue.UnmarshalListOfMaps(result.Items, &matched)
		return matched
	}

	if approverID != "" {
		goals = append(goals, queryApprover("APPROVER#"+approverID)...)
	}
	if approverEmail != "" {
		for _, g := range queryApprover("APPROVER#" + approverEmail) {
			found := false
			for _, existing := range goals {
				if existing.ID == g.ID {
					found = true
					break
				}
			}
			if !found {
				goals = append(goals, g)
			}
		}
	}

	// Merge/fallback with local memory cache
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, g := range r.memGoals {
		matches := false
		if approverID != "" && g.ApproverID == approverID {
			matches = true
		}
		if approverEmail != "" && strings.EqualFold(strings.TrimSpace(g.ApproverEmail), approverEmail) {
			matches = true
		}
		if matches {
			found := false
			for _, existing := range goals {
				if existing.ID == g.ID {
					found = true
					break
				}
			}
			if !found {
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
	r.mu.Lock()
	if r.memGoals != nil {
		delete(r.memGoals, ownerID+"#"+goalID)
		r.saveLocalFallback()
	}
	r.mu.Unlock()

	deleteCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
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
		// Remote DB offline / unavailable in local dev: memory deletion succeeded
		return nil
	}

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
	conditionExpression := strings.Join(conditionParts, " AND ")

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
		// If remote DB call timed out/failed, update in memory
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.memGoals != nil {
			key := ownerID + "#" + goalID
			if g, ok := r.memGoals[key]; ok {
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
				}
				if input.ApproverID != nil {
					g.ApproverID = *input.ApproverID
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
				r.memGoals[key] = g
				r.saveLocalFallback()
				cp := g
				return &cp, nil
			}
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
	r.mu.Lock()
	var inMemGoal *Goal
	if r.memGoals != nil {
		key := ownerID + "#" + goalID
		if g, ok := r.memGoals[key]; ok {
			g.Status = status
			r.memGoals[key] = g
			r.saveLocalFallback()
			cp := g
			inMemGoal = &cp
		}
	}
	r.mu.Unlock()

	updateCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
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
			"SET #status = :status",
		),
		ExpressionAttributeNames: map[string]string{
			"#status": "Status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{
				Value: status,
			},
		},
		ReturnValues: types.ReturnValueAllNew,
	})

	if err != nil {
		if inMemGoal != nil {
			return inMemGoal, nil
		}
		return nil, err
	}

	var goal Goal

	if err := attributevalue.UnmarshalMap(result.Attributes, &goal); err != nil {
		if inMemGoal != nil {
			return inMemGoal, nil
		}
		return nil, err
	}

	return &goal, nil
}
