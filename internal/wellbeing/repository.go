package wellbeing

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	wellbeingDayPrefix = "WELLBEING#DAY#"
	wellbeingAllTimeSK = "WELLBEING#ALLTIME"
	wellbeingRulesSK   = "WELLBEING#RULES"
)

type Repository interface {
	UpsertDailyDevice(ctx context.Context, record DailyDeviceRecord) (DailyDeviceRecord, error)
	ListDailyRecords(ctx context.Context, userID, startDate, endDate string) ([]DailyDeviceRecord, error)
	GetAllTimeRecord(ctx context.Context, userID string) (*AllTimeRecord, error)
	GetRulesRecord(ctx context.Context, userID string) (*WellbeingRulesRecord, error)
	SaveRulesRecord(ctx context.Context, record WellbeingRulesRecord) error
}

type DynamoRepository struct {
	db         *dynamodb.Client
	tableName  string
	mu         sync.RWMutex
	memDaily   map[string]map[string]DailyDeviceRecord // userID -> SK -> record
	memAllTime map[string]AllTimeRecord                // userID -> AllTimeRecord
	memRules   map[string]WellbeingRulesRecord         // userID -> Rules
}

func NewDynamoRepository(db *dynamodb.Client, tableName string) *DynamoRepository {
	return &DynamoRepository{
		db:         db,
		tableName:  tableName,
		memDaily:   make(map[string]map[string]DailyDeviceRecord),
		memAllTime: make(map[string]AllTimeRecord),
		memRules:   make(map[string]WellbeingRulesRecord),
	}
}

func FormatDailySK(date, deviceID string) string {
	return wellbeingDayPrefix + date + "#DEV#" + deviceID
}

// UpsertDailyDevice merges incoming cumulative usage monotonically (max per metric)
// and propagates any positive delta into the user's AllTimeRecord.
func (r *DynamoRepository) UpsertDailyDevice(ctx context.Context, incoming DailyDeviceRecord) (DailyDeviceRecord, error) {
	pk := "USER#" + incoming.UserID
	sk := FormatDailySK(incoming.Date, incoming.DeviceID)
	incoming.PK = pk
	incoming.SK = sk

	existing := r.getDailyRecord(ctx, incoming.UserID, pk, sk)
	dbAllTime := r.fetchAllTimeFromDBIfNotCached(ctx, incoming.UserID, pk)

	r.mu.Lock()
	if r.memDaily[incoming.UserID] == nil {
		r.memDaily[incoming.UserID] = make(map[string]DailyDeviceRecord)
	}
	if memExisting, ok := r.memDaily[incoming.UserID][sk]; ok {
		existing = &memExisting
	}

	merged, deltas := mergeMonotonicDaily(existing, incoming)
	r.memDaily[incoming.UserID][sk] = merged

	allTime, hasMemAll := r.memAllTime[incoming.UserID]
	if hasMemAll {
		allTime = cloneAllTime(allTime)
	} else if dbAllTime != nil {
		allTime = *dbAllTime
	} else {
		allTime = AllTimeRecord{
			PK:      pk,
			SK:      wellbeingAllTimeSK,
			UserID:  incoming.UserID,
			Apps:    make(map[string]AppUsageEntry),
			Domains: make(map[string]DomainUsageEntry),
		}
	}
	updatedAllTime := applyDeltasToAllTime(allTime, incoming.UserID, pk, deltas, incoming.UpdatedAt)
	r.memAllTime[incoming.UserID] = cloneAllTime(updatedAllTime)

	mergedCopy := cloneDaily(merged)
	allTimeCopy := cloneAllTime(updatedAllTime)
	r.mu.Unlock()

	if r.db != nil {
		r.persistItem(ctx, mergedCopy)
		r.persistItem(ctx, allTimeCopy)
	}

	return mergedCopy, nil
}

func (r *DynamoRepository) ListDailyRecords(ctx context.Context, userID, startDate, endDate string) ([]DailyDeviceRecord, error) {
	pk := "USER#" + userID
	startSK := wellbeingDayPrefix + startDate
	endSK := wellbeingDayPrefix + endDate + "~" // '~' sorts after '#DEV#...' in ASCII

	mergedMap := make(map[string]DailyDeviceRecord)

	if r.db != nil {
		var lastKey map[string]types.AttributeValue
		for {
			qCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
			out, err := r.db.Query(qCtx, &dynamodb.QueryInput{
				TableName:              aws.String(r.tableName),
				KeyConditionExpression: aws.String("PK = :pk AND SK BETWEEN :start AND :end"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":pk":    &types.AttributeValueMemberS{Value: pk},
					":start": &types.AttributeValueMemberS{Value: startSK},
					":end":   &types.AttributeValueMemberS{Value: endSK},
				},
				ExclusiveStartKey: lastKey,
			})
			cancel()
			if err != nil {
				log.Printf("[WellbeingRepo] Remote ListDailyRecords notice: %v (using local cache)", err)
				break
			}
			for _, item := range out.Items {
				var rec DailyDeviceRecord
				if unmarshalErr := attributevalue.UnmarshalMap(item, &rec); unmarshalErr == nil && rec.SK != "" {
					mergedMap[rec.SK] = rec
				}
			}
			if len(out.LastEvaluatedKey) == 0 {
				break
			}
			lastKey = out.LastEvaluatedKey
		}
	}

	r.mu.RLock()
	if userMem, ok := r.memDaily[userID]; ok {
		for sk, rec := range userMem {
			if rec.Date >= startDate && rec.Date <= endDate {
				if existing, exists := mergedMap[sk]; !exists || rec.UpdatedAt >= existing.UpdatedAt {
					mergedMap[sk] = rec
				}
			}
		}
	}
	r.mu.RUnlock()

	results := make([]DailyDeviceRecord, 0, len(mergedMap))
	for _, rec := range mergedMap {
		results = append(results, rec)
	}
	return results, nil
}

func (r *DynamoRepository) GetAllTimeRecord(ctx context.Context, userID string) (*AllTimeRecord, error) {
	pk := "USER#" + userID

	r.mu.RLock()
	memRec, hasMem := r.memAllTime[userID]
	r.mu.RUnlock()

	if r.db != nil {
		gCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		out, err := r.db.GetItem(gCtx, &dynamodb.GetItemInput{
			TableName: aws.String(r.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: pk},
				"SK": &types.AttributeValueMemberS{Value: wellbeingAllTimeSK},
			},
		})
		cancel()
		if err == nil && out.Item != nil {
			var dbRec AllTimeRecord
			if uErr := attributevalue.UnmarshalMap(out.Item, &dbRec); uErr == nil {
				if !hasMem || dbRec.TotalScreenSeconds >= memRec.TotalScreenSeconds {
					cp := cloneAllTime(dbRec)
					return &cp, nil
				}
			}
		}
	}

	if hasMem {
		cp := cloneAllTime(memRec)
		return &cp, nil
	}

	return &AllTimeRecord{
		PK:      pk,
		SK:      wellbeingAllTimeSK,
		UserID:  userID,
		Apps:    make(map[string]AppUsageEntry),
		Domains: make(map[string]DomainUsageEntry),
	}, nil
}

func (r *DynamoRepository) getDailyRecord(ctx context.Context, userID, pk, sk string) *DailyDeviceRecord {
	r.mu.RLock()
	if userMem, ok := r.memDaily[userID]; ok {
		if rec, found := userMem[sk]; found {
			r.mu.RUnlock()
			cp := cloneDaily(rec)
			return &cp
		}
	}
	r.mu.RUnlock()

	if r.db != nil {
		gCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		out, err := r.db.GetItem(gCtx, &dynamodb.GetItemInput{
			TableName: aws.String(r.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: pk},
				"SK": &types.AttributeValueMemberS{Value: sk},
			},
		})
		cancel()
		if err == nil && out.Item != nil {
			var rec DailyDeviceRecord
			if uErr := attributevalue.UnmarshalMap(out.Item, &rec); uErr == nil {
				return &rec
			}
		}
	}
	return nil
}

func (r *DynamoRepository) fetchAllTimeFromDBIfNotCached(ctx context.Context, userID, pk string) *AllTimeRecord {
	r.mu.RLock()
	_, ok := r.memAllTime[userID]
	r.mu.RUnlock()
	if ok || r.db == nil {
		return nil
	}
	gCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	out, err := r.db.GetItem(gCtx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: wellbeingAllTimeSK},
		},
	})
	cancel()
	if err == nil && out.Item != nil {
		var rec AllTimeRecord
		if uErr := attributevalue.UnmarshalMap(out.Item, &rec); uErr == nil {
			if rec.Apps == nil {
				rec.Apps = make(map[string]AppUsageEntry)
			}
			if rec.Domains == nil {
				rec.Domains = make(map[string]DomainUsageEntry)
			}
			return &rec
		}
	}
	return nil
}

func (r *DynamoRepository) persistItem(ctx context.Context, v any) {
	item, err := attributevalue.MarshalMap(v)
	if err != nil {
		log.Printf("[WellbeingRepo] Marshal error: %v", err)
		return
	}
	putCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	if _, err := r.db.PutItem(putCtx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	}); err != nil {
		log.Printf("[WellbeingRepo] Remote PutItem notice: %v (cached in local memory)", err)
	}
}

type dailyDeltas struct {
	screenSeconds int64
	apps          map[string]AppUsageEntry
	domains       map[string]DomainUsageEntry
}

func mergeMonotonicDaily(existing *DailyDeviceRecord, incoming DailyDeviceRecord) (DailyDeviceRecord, dailyDeltas) {
	deltas := dailyDeltas{
		apps:    make(map[string]AppUsageEntry),
		domains: make(map[string]DomainUsageEntry),
	}

	mergedApps := make(map[string]AppUsageEntry)
	mergedDomains := make(map[string]DomainUsageEntry)

	var prevTotal int64
	if existing != nil {
		prevTotal = existing.TotalScreenSeconds
		for k, v := range existing.Apps {
			mergedApps[k] = v
		}
		for k, v := range existing.Domains {
			mergedDomains[k] = v
		}
	}

	newTotal := incoming.TotalScreenSeconds
	if newTotal < prevTotal {
		newTotal = prevTotal
	}
	deltas.screenSeconds = newTotal - prevTotal

	for k, inApp := range incoming.Apps {
		prev, hasPrev := mergedApps[k]
		maxSec := maxInt64(prev.Seconds, inApp.Seconds)
		maxOpens := maxInt64(prev.Opens, inApp.Opens)
		disp := inApp.DisplayName
		if disp == "" {
			disp = prev.DisplayName
		}
		mergedApps[k] = AppUsageEntry{
			AppID:       k,
			DisplayName: disp,
			Seconds:     maxSec,
			Opens:       maxOpens,
		}
		dSec := maxSec - prev.Seconds
		dOpens := maxOpens - prev.Opens
		if !hasPrev {
			dSec = maxSec
			dOpens = maxOpens
		}
		if dSec > 0 || dOpens > 0 {
			deltas.apps[k] = AppUsageEntry{
				AppID:       k,
				DisplayName: disp,
				Seconds:     dSec,
				Opens:       dOpens,
			}
		}
	}

	for k, inDom := range incoming.Domains {
		prev, hasPrev := mergedDomains[k]
		maxSec := maxInt64(prev.Seconds, inDom.Seconds)
		maxOpens := maxInt64(prev.Opens, inDom.Opens)
		maxBlocked := maxInt64(prev.BlockedAttempts, inDom.BlockedAttempts)

		mergedDomains[k] = DomainUsageEntry{
			Domain:          k,
			Seconds:         maxSec,
			Opens:           maxOpens,
			BlockedAttempts: maxBlocked,
		}
		dSec := maxSec - prev.Seconds
		dOpens := maxOpens - prev.Opens
		dBlocked := maxBlocked - prev.BlockedAttempts
		if !hasPrev {
			dSec = maxSec
			dOpens = maxOpens
			dBlocked = maxBlocked
		}
		if dSec > 0 || dOpens > 0 || dBlocked > 0 {
			deltas.domains[k] = DomainUsageEntry{
				Domain:          k,
				Seconds:         dSec,
				Opens:           dOpens,
				BlockedAttempts: dBlocked,
			}
		}
	}

	merged := DailyDeviceRecord{
		PK:                 incoming.PK,
		SK:                 incoming.SK,
		UserID:             incoming.UserID,
		Date:               incoming.Date,
		DeviceID:           incoming.DeviceID,
		ClientType:         incoming.ClientType,
		Platform:           incoming.Platform,
		TotalScreenSeconds: newTotal,
		Apps:               mergedApps,
		Domains:            mergedDomains,
		UpdatedAt:          incoming.UpdatedAt,
	}
	return merged, deltas
}

func applyDeltasToAllTime(allTime AllTimeRecord, userID, pk string, d dailyDeltas, updatedAt string) AllTimeRecord {
	allTime.PK = pk
	allTime.SK = wellbeingAllTimeSK
	allTime.UserID = userID
	allTime.TotalScreenSeconds += d.screenSeconds
	allTime.UpdatedAt = updatedAt

	if allTime.Apps == nil {
		allTime.Apps = make(map[string]AppUsageEntry)
	}
	for k, delta := range d.apps {
		cur := allTime.Apps[k]
		cur.AppID = k
		if delta.DisplayName != "" {
			cur.DisplayName = delta.DisplayName
		} else if cur.DisplayName == "" {
			cur.DisplayName = k
		}
		cur.Seconds += delta.Seconds
		cur.Opens += delta.Opens
		allTime.Apps[k] = cur
	}

	if allTime.Domains == nil {
		allTime.Domains = make(map[string]DomainUsageEntry)
	}
	for k, delta := range d.domains {
		cur := allTime.Domains[k]
		cur.Domain = k
		cur.Seconds += delta.Seconds
		cur.Opens += delta.Opens
		cur.BlockedAttempts += delta.BlockedAttempts
		allTime.Domains[k] = cur
	}

	return allTime
}

func cloneDaily(src DailyDeviceRecord) DailyDeviceRecord {
	cp := src
	cp.Apps = make(map[string]AppUsageEntry, len(src.Apps))
	for k, v := range src.Apps {
		cp.Apps[k] = v
	}
	cp.Domains = make(map[string]DomainUsageEntry, len(src.Domains))
	for k, v := range src.Domains {
		cp.Domains[k] = v
	}
	return cp
}

func cloneAllTime(src AllTimeRecord) AllTimeRecord {
	cp := src
	cp.Apps = make(map[string]AppUsageEntry, len(src.Apps))
	for k, v := range src.Apps {
		cp.Apps[k] = v
	}
	cp.Domains = make(map[string]DomainUsageEntry, len(src.Domains))
	for k, v := range src.Domains {
		cp.Domains[k] = v
	}
	return cp
}

func (r *DynamoRepository) GetRulesRecord(ctx context.Context, userID string) (*WellbeingRulesRecord, error) {
	pk := "USER#" + userID

	r.mu.RLock()
	memRec, hasMem := r.memRules[userID]
	r.mu.RUnlock()

	if hasMem {
		cp := cloneRules(memRec)
		return &cp, nil
	}

	if r.db != nil {
		gCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		out, err := r.db.GetItem(gCtx, &dynamodb.GetItemInput{
			TableName: aws.String(r.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: pk},
				"SK": &types.AttributeValueMemberS{Value: wellbeingRulesSK},
			},
		})
		cancel()
		if err == nil && out.Item != nil {
			var rec WellbeingRulesRecord
			if uErr := attributevalue.UnmarshalMap(out.Item, &rec); uErr == nil {
				if rec.Limits == nil {
					rec.Limits = make(map[string]TimeLimitRule)
				}
				r.mu.Lock()
				r.memRules[userID] = rec
				r.mu.Unlock()
				cp := cloneRules(rec)
				return &cp, nil
			}
		}
	}

	return &WellbeingRulesRecord{
		PK:     pk,
		SK:     wellbeingRulesSK,
		UserID: userID,
		Limits: make(map[string]TimeLimitRule),
		Schedule: FocusScheduleConfig{
			Name:      "Wind-Down Focus Hours",
			StartTime: "23:00",
			EndTime:   "07:00",
		},
	}, nil
}

func (r *DynamoRepository) SaveRulesRecord(ctx context.Context, record WellbeingRulesRecord) error {
	pk := "USER#" + record.UserID
	record.PK = pk
	record.SK = wellbeingRulesSK
	if record.Limits == nil {
		record.Limits = make(map[string]TimeLimitRule)
	}

	r.mu.Lock()
	r.memRules[record.UserID] = cloneRules(record)
	r.mu.Unlock()

	if r.db != nil {
		r.persistItem(ctx, record)
	}
	return nil
}

func cloneRules(src WellbeingRulesRecord) WellbeingRulesRecord {
	cp := src
	cp.Limits = make(map[string]TimeLimitRule, len(src.Limits))
	for k, v := range src.Limits {
		cp.Limits[k] = v
	}
	cp.Schedule.Domains = append([]string(nil), src.Schedule.Domains...)
	cp.Schedule.Apps = append([]string(nil), src.Schedule.Apps...)
	return cp
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

var _ = fmt.Sprintf
