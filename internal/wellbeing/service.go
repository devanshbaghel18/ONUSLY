package wellbeing

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

// LimitNotifier is invoked when newly ingested wellbeing telemetry causes one or more
// enabled daily time limits to become exceeded for the user's current local day.
type LimitNotifier interface {
	NotifyLimitReached(ctx context.Context, userID string, exceeded []TimeLimitRule)
}

type Service struct {
	repo          Repository
	nowFn         func() time.Time
	userLocksMu   sync.Mutex
	userLocks     map[string]*sync.Mutex
	limitNotifier LimitNotifier
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:      repo,
		userLocks: make(map[string]*sync.Mutex),
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) lockUserRules(userID string) func() {
	s.userLocksMu.Lock()
	if s.userLocks == nil {
		s.userLocks = make(map[string]*sync.Mutex)
	}
	mu, ok := s.userLocks[userID]
	if !ok {
		mu = &sync.Mutex{}
		s.userLocks[userID] = mu
	}
	s.userLocksMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// SetLimitNotifier attaches an optional callback for real-time daily limit exceeded pushes.
func (s *Service) SetLimitNotifier(notifier LimitNotifier) {
	s.limitNotifier = notifier
}

var (
	androidPkgPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
	desktopAppPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
)

func classifyAppTarget(raw string) (kind, cleanID string) {
	cleanID = strings.ToLower(strings.TrimSpace(raw))
	if androidPkgPattern.MatchString(cleanID) && !strings.HasSuffix(cleanID, ".exe") && !strings.HasSuffix(cleanID, ".app") && !strings.HasSuffix(cleanID, ".desktop") {
		return "android_package", cleanID
	}
	return "desktop_app", cleanID
}

func isProtectedWellbeingDomain(dom string) bool {
	return dom == "" || dom == "localhost" || dom == "127.0.0.1" || dom == "onusly.com" || strings.HasSuffix(dom, ".onusly.com") || dom == "onusly.app" || strings.HasSuffix(dom, ".onusly.app")
}

func appendAppTargets(dst []goals.ApplicationTarget, rawIDs []string) []goals.ApplicationTarget {
	for _, raw := range rawIDs {
		kind, clean := classifyAppTarget(raw)
		if clean == "" {
			continue
		}
		dst = append(dst, goals.ApplicationTarget{
			Kind:       kind,
			Identifier: clean,
		})
	}
	return dst
}

var validClientTypes = map[string]bool{
	"android":       true,
	"extension":     true,
	"desktop_agent": true,
	"desktop-agent": true,
	"web":           true,
}

func normalizeClientType(raw string) string {
	c := strings.ToLower(strings.TrimSpace(raw))
	if c == "desktop-agent" {
		return "desktop_agent"
	}
	return c
}

// NormalizeDomain strips schemes, www., paths, queries, and ports to preserve privacy and deduplicate hosts.
func NormalizeDomain(raw string) string {
	d := strings.TrimSpace(strings.ToLower(raw))
	if d == "" {
		return ""
	}
	if strings.Contains(d, "://") {
		if u, err := url.Parse(d); err == nil && u.Hostname() != "" {
			d = strings.ToLower(u.Hostname())
		} else {
			d = strings.SplitN(d, "://", 2)[1]
		}
	}
	for _, sep := range []string{"/", "?", "#"} {
		if idx := strings.Index(d, sep); idx != -1 {
			d = d[:idx]
		}
	}
	if idx := strings.Index(d, ":"); idx != -1 && !strings.HasPrefix(d, "[") {
		d = d[:idx]
	}
	d = strings.Trim(d, ".")
	d = strings.TrimPrefix(d, "www.")
	return d
}

func clampNonNegative(v, max int64) int64 {
	if v < 0 {
		return 0
	}
	if max > 0 && v > max {
		return max
	}
	return v
}

func (s *Service) Ingest(ctx context.Context, userID string, req IngestRequest) (*IngestResponse, error) {
	dateStr := strings.TrimSpace(req.Date)
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		return nil, ErrInvalidDate
	}

	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" || len(deviceID) > 128 {
		return nil, ErrInvalidDeviceID
	}

	clientType := normalizeClientType(req.ClientType)
	if !validClientTypes[clientType] {
		return nil, ErrInvalidClientType
	}

	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform == "" {
		platform = clientType
	}

	appsMap := make(map[string]AppUsageEntry)
	var sumAppSeconds int64
	for _, entry := range req.Apps {
		appID := strings.TrimSpace(entry.AppID)
		if appID == "" {
			continue
		}
		sec := clampNonNegative(entry.Seconds, MaxSecondsPerDay)
		opens := clampNonNegative(entry.Opens, 50000)
		if sec == 0 && opens == 0 {
			continue
		}
		disp := strings.TrimSpace(entry.DisplayName)
		if disp == "" {
			disp = appID
		}
		existing := appsMap[appID]
		existing.AppID = appID
		existing.DisplayName = disp
		existing.Seconds = clampNonNegative(existing.Seconds+sec, MaxSecondsPerDay)
		existing.Opens += opens
		appsMap[appID] = existing
	}
	for _, a := range appsMap {
		sumAppSeconds += a.Seconds
	}

	domainsMap := make(map[string]DomainUsageEntry)
	var sumWebSeconds int64
	for _, entry := range req.Domains {
		dom := NormalizeDomain(entry.Domain)
		if dom == "" || dom == "localhost" || dom == "127.0.0.1" {
			continue
		}
		sec := clampNonNegative(entry.Seconds, MaxSecondsPerDay)
		opens := clampNonNegative(entry.Opens, 50000)
		blocked := clampNonNegative(entry.BlockedAttempts, 50000)
		if sec == 0 && opens == 0 && blocked == 0 {
			continue
		}
		existing := domainsMap[dom]
		existing.Domain = dom
		existing.Seconds = clampNonNegative(existing.Seconds+sec, MaxSecondsPerDay)
		existing.Opens += opens
		existing.BlockedAttempts += blocked
		domainsMap[dom] = existing
	}
	for _, d := range domainsMap {
		sumWebSeconds += d.Seconds
	}

	totalScreen := clampNonNegative(req.TotalScreenSeconds, MaxSecondsPerDay)
	// Ensure totalScreen is at least the max of reported total, summed apps, or summed web time (capped at 24h)
	derivedTotal := sumAppSeconds
	if clientType == "extension" && sumWebSeconds > derivedTotal {
		derivedTotal = sumWebSeconds
	}
	if totalScreen < derivedTotal {
		totalScreen = clampNonNegative(derivedTotal, MaxSecondsPerDay)
	}

	rec := DailyDeviceRecord{
		UserID:             userID,
		Date:               dateStr,
		DeviceID:           deviceID,
		ClientType:         clientType,
		Platform:           platform,
		TotalScreenSeconds: totalScreen,
		Apps:               appsMap,
		Domains:            domainsMap,
		UpdatedAt:          s.nowFn().Format(time.RFC3339),
	}

	var preExceeded map[string]bool
	var effectiveOffset int
	hasEffectiveOffset := false
	rulesRec, errRules := s.repo.GetRulesRecord(ctx, userID)
	if errRules == nil && rulesRec != nil {
		if req.TimezoneOffsetMinutes != nil && *req.TimezoneOffsetMinutes >= -840 && *req.TimezoneOffsetMinutes <= 840 {
			effectiveOffset = *req.TimezoneOffsetMinutes
			hasEffectiveOffset = true
			if rulesRec.LastTimezoneOffsetMinutes == nil || *rulesRec.LastTimezoneOffsetMinutes != effectiveOffset {
				unlockRules := s.lockUserRules(userID)
				latest, gErr := s.repo.GetRulesRecord(ctx, userID)
				if gErr == nil && latest != nil {
					tz := effectiveOffset
					latest.LastTimezoneOffsetMinutes = &tz
					_ = s.repo.SaveRulesRecord(ctx, *latest)
				}
				unlockRules()
			}
		} else {
			effectiveOffset = resolveEffectiveTzOffset(rulesRec, nil, false)
			hasEffectiveOffset = true
		}
		if s.limitNotifier != nil && len(rulesRec.Limits) > 0 && hasEffectiveOffset {
			todayDate := s.nowFn().Add(time.Duration(effectiveOffset) * time.Minute).Format("2006-01-02")
			if dateStr == todayDate {
				appSec, domSec := s.computeTodayUsageMaps(ctx, userID, effectiveOffset)
				preExceeded = computeExceededMap(rulesRec.Limits, appSec, domSec)
			}
		}
	}

	merged, err := s.repo.UpsertDailyDevice(ctx, rec)
	if err != nil {
		return nil, err
	}

	if s.limitNotifier != nil && preExceeded != nil {
		latestRules, lErr := s.repo.GetRulesRecord(ctx, userID)
		if lErr == nil && latestRules != nil && len(latestRules.Limits) > 0 {
			appSec, domSec := s.computeTodayUsageMaps(ctx, userID, effectiveOffset)
			evaluated := evaluateLimits(latestRules.Limits, appSec, domSec)
			var newlyExceeded []TimeLimitRule
			for _, rule := range evaluated {
				if rule.Exceeded && !preExceeded[rule.ID] {
					newlyExceeded = append(newlyExceeded, rule)
				}
			}
			if len(newlyExceeded) > 0 {
				s.limitNotifier.NotifyLimitReached(ctx, userID, newlyExceeded)
			}
		}
	}

	return &IngestResponse{
		Status:             "ok",
		Date:               merged.Date,
		DeviceID:           merged.DeviceID,
		TotalScreenSeconds: merged.TotalScreenSeconds,
		AppsCount:          len(merged.Apps),
		DomainsCount:       len(merged.Domains),
	}, nil
}

func (s *Service) GetSummary(
	ctx context.Context,
	userID string,
	rangeParam string,
	tzOffsetMinutes int,
	deviceFilter string,
) (*SummaryResponse, error) {
	rng := strings.ToLower(strings.TrimSpace(rangeParam))
	if rng == "" {
		rng = "today"
	}
	if rng != "today" && rng != "yesterday" && rng != "7d" && rng != "all" {
		return nil, ErrInvalidRange
	}

	// Calculate user's local calendar dates using tzOffsetMinutes (JS getTimezoneOffset semantics or positive offset; clamp safely)
	if tzOffsetMinutes < -840 || tzOffsetMinutes > 840 {
		tzOffsetMinutes = 0
	}
	localNow := s.nowFn().Add(time.Duration(tzOffsetMinutes) * time.Minute)
	todayDate := localNow.Format("2006-01-02")
	yesterdayDate := localNow.AddDate(0, 0, -1).Format("2006-01-02")
	dayBeforeYesterdayDate := localNow.AddDate(0, 0, -2).Format("2006-01-02")
	weekStartDate := localNow.AddDate(0, 0, -6).Format("2006-01-02")

	var startDate, endDate string
	switch rng {
	case "today":
		startDate, endDate = todayDate, todayDate
	case "yesterday":
		startDate, endDate = yesterdayDate, yesterdayDate
	case "7d":
		startDate, endDate = weekStartDate, todayDate
	case "all":
		startDate, endDate = "2020-01-01", todayDate
	}

	// Always fetch at least past 7 days so the 7-day trend chart & yesterday comparison work seamlessly
	fetchStart := startDate
	if weekStartDate < fetchStart {
		fetchStart = weekStartDate
	}

	records, err := s.repo.ListDailyRecords(ctx, userID, fetchStart, todayDate)
	if err != nil {
		return nil, err
	}

	deviceFilter = strings.TrimSpace(deviceFilter)
	if deviceFilter != "" {
		filtered := records[:0]
		for _, r := range records {
			if r.DeviceID == deviceFilter || r.ClientType == deviceFilter {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}

	// Build 7-day daily trend map
	dailyPointsMap := make(map[string]*DailyPoint)
	for i := 6; i >= 0; i-- {
		d := localNow.AddDate(0, 0, -i).Format("2006-01-02")
		dailyPointsMap[d] = &DailyPoint{Date: d}
	}

	for _, rec := range records {
		dp, inWeek := dailyPointsMap[rec.Date]
		if !inWeek {
			continue
		}
		var recAppSec, recWebSec, recOpens, recBlocked int64
		for _, a := range rec.Apps {
			recAppSec += a.Seconds
		}
		for _, d := range rec.Domains {
			recWebSec += d.Seconds
			recOpens += d.Opens
			recBlocked += d.BlockedAttempts
		}
		dp.TotalSeconds += rec.TotalScreenSeconds
		dp.AppSeconds += recAppSec
		dp.WebSeconds += recWebSec
		dp.WebsiteOpens += recOpens
		dp.BlockedAttempts += recBlocked
	}

	dailySeries := make([]DailyPoint, 0, 7)
	for i := 6; i >= 0; i-- {
		d := localNow.AddDate(0, 0, -i).Format("2006-01-02")
		dailySeries = append(dailySeries, *dailyPointsMap[d])
	}

	var totalScreenSec, totalAppSec, totalWebSec, totalOpens, totalBlocked int64
	appAgg := make(map[string]*AggregatedApp)
	appPlatforms := make(map[string]map[string]bool)
	webAgg := make(map[string]*AggregatedWebsite)
	webPlatforms := make(map[string]map[string]bool)
	devAgg := make(map[string]*DeviceSummary)

	if rng == "all" && deviceFilter == "" {
		allTime, errAll := s.repo.GetAllTimeRecord(ctx, userID)
		if errAll == nil && allTime != nil {
			totalScreenSec = allTime.TotalScreenSeconds
			for k, a := range allTime.Apps {
				totalAppSec += a.Seconds
				appAgg[k] = &AggregatedApp{
					AppID:       k,
					DisplayName: a.DisplayName,
					Seconds:     a.Seconds,
					Opens:       a.Opens,
					Platforms:   []string{},
				}
			}
			for k, d := range allTime.Domains {
				totalWebSec += d.Seconds
				totalOpens += d.Opens
				totalBlocked += d.BlockedAttempts
				webAgg[k] = &AggregatedWebsite{
					Domain:          k,
					Seconds:         d.Seconds,
					Opens:           d.Opens,
					BlockedAttempts: d.BlockedAttempts,
					Platforms:       []string{},
				}
			}
		}
		// Populate device list and platform tags from records within fetch range
		for _, rec := range records {
			mergeDeviceSummary(devAgg, rec)
			if rec.Platform == "" {
				continue
			}
			for k := range rec.Apps {
				if appPlatforms[k] == nil {
					appPlatforms[k] = make(map[string]bool)
				}
				appPlatforms[k][rec.Platform] = true
			}
			for k := range rec.Domains {
				if webPlatforms[k] == nil {
					webPlatforms[k] = make(map[string]bool)
				}
				webPlatforms[k][rec.Platform] = true
			}
		}
	} else {
		for _, rec := range records {
			if rec.Date < startDate || rec.Date > endDate {
				continue
			}
			totalScreenSec += rec.TotalScreenSeconds
			mergeDeviceSummary(devAgg, rec)

			for k, a := range rec.Apps {
				totalAppSec += a.Seconds
				agg, exists := appAgg[k]
				if !exists {
					agg = &AggregatedApp{
						AppID:       k,
						DisplayName: a.DisplayName,
					}
					appAgg[k] = agg
					appPlatforms[k] = make(map[string]bool)
				}
				if a.DisplayName != "" {
					agg.DisplayName = a.DisplayName
				}
				agg.Seconds += a.Seconds
				agg.Opens += a.Opens
				if rec.Platform != "" {
					appPlatforms[k][rec.Platform] = true
				}
			}

			for k, d := range rec.Domains {
				totalWebSec += d.Seconds
				totalOpens += d.Opens
				totalBlocked += d.BlockedAttempts
				agg, exists := webAgg[k]
				if !exists {
					agg = &AggregatedWebsite{
						Domain: k,
					}
					webAgg[k] = agg
					webPlatforms[k] = make(map[string]bool)
				}
				agg.Seconds += d.Seconds
				agg.Opens += d.Opens
				agg.BlockedAttempts += d.BlockedAttempts
				if rec.Platform != "" {
					webPlatforms[k][rec.Platform] = true
				}
			}
		}
	}

	var compDelta int64
	if rng == "today" {
		todayTot := dailyPointsMap[todayDate].TotalSeconds
		yestTot := dailyPointsMap[yesterdayDate].TotalSeconds
		compDelta = todayTot - yestTot
	} else if rng == "yesterday" {
		yestTot := dailyPointsMap[yesterdayDate].TotalSeconds
		prevTot := dailyPointsMap[dayBeforeYesterdayDate].TotalSeconds
		compDelta = yestTot - prevTot
	}

	topApps := make([]AggregatedApp, 0, len(appAgg))
	for k, agg := range appAgg {
		agg.Platforms = sortedKeys(appPlatforms[k])
		topApps = append(topApps, *agg)
	}
	sort.Slice(topApps, func(i, j int) bool {
		if topApps[i].Seconds == topApps[j].Seconds {
			return topApps[i].Opens > topApps[j].Opens
		}
		return topApps[i].Seconds > topApps[j].Seconds
	})

	topWebsites := make([]AggregatedWebsite, 0, len(webAgg))
	for k, agg := range webAgg {
		agg.Platforms = sortedKeys(webPlatforms[k])
		topWebsites = append(topWebsites, *agg)
	}
	sort.Slice(topWebsites, func(i, j int) bool {
		if topWebsites[i].Seconds == topWebsites[j].Seconds {
			if topWebsites[i].Opens == topWebsites[j].Opens {
				return topWebsites[i].BlockedAttempts > topWebsites[j].BlockedAttempts
			}
			return topWebsites[i].Opens > topWebsites[j].Opens
		}
		return topWebsites[i].Seconds > topWebsites[j].Seconds
	})

	devices := make([]DeviceSummary, 0, len(devAgg))
	for _, dev := range devAgg {
		devices = append(devices, *dev)
	}
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].TotalScreenSeconds > devices[j].TotalScreenSeconds
	})

	return &SummaryResponse{
		Range:                  rng,
		StartDate:              startDate,
		EndDate:                endDate,
		TotalScreenSeconds:     totalScreenSec,
		TotalAppSeconds:        totalAppSec,
		TotalWebSeconds:        totalWebSec,
		ComparisonDeltaSeconds: compDelta,
		TotalWebsiteOpens:      totalOpens,
		TotalBlockedAttempts:   totalBlocked,
		DailySeries:            dailySeries,
		TopApps:                topApps,
		TopWebsites:            topWebsites,
		Devices:                devices,
	}, nil
}

func mergeDeviceSummary(devAgg map[string]*DeviceSummary, rec DailyDeviceRecord) {
	dev, exists := devAgg[rec.DeviceID]
	if !exists {
		dev = &DeviceSummary{
			DeviceID:   rec.DeviceID,
			ClientType: rec.ClientType,
			Platform:   rec.Platform,
		}
		devAgg[rec.DeviceID] = dev
	}
	dev.TotalScreenSeconds += rec.TotalScreenSeconds
	for _, d := range rec.Domains {
		dev.WebsiteOpens += d.Opens
		dev.BlockedAttempts += d.BlockedAttempts
	}
	if rec.UpdatedAt > dev.LastUpdatedAt {
		dev.LastUpdatedAt = rec.UpdatedAt
	}
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return []string{}
	}
	res := make([]string, 0, len(m))
	for k := range m {
		res = append(res, k)
	}
	sort.Strings(res)
	return res
}

func formatRuleID(targetType, cleanTarget string) string {
	return targetType + ":" + cleanTarget
}

func parseHHMMMinutes(raw string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return 0, false
	}
	hh, errH := strconv.Atoi(parts[0])
	mm, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, false
	}
	return hh*60 + mm, true
}

func IsScheduleActiveAt(cfg FocusScheduleConfig, localNow time.Time) bool {
	if !cfg.Enabled || (len(cfg.Domains) == 0 && len(cfg.Apps) == 0) {
		return false
	}
	startMin, okS := parseHHMMMinutes(cfg.StartTime)
	endMin, okE := parseHHMMMinutes(cfg.EndTime)
	if !okS || !okE || startMin == endMin {
		return false
	}
	curMin := localNow.Hour()*60 + localNow.Minute()
	if startMin < endMin {
		return curMin >= startMin && curMin < endMin
	}
	// Overnight window (e.g. 23:00 -> 07:00)
	return curMin >= startMin || curMin < endMin
}

func (s *Service) computeTodayUsageMaps(ctx context.Context, userID string, tzOffsetMinutes int) (map[string]int64, map[string]int64) {
	if tzOffsetMinutes < -840 || tzOffsetMinutes > 840 {
		tzOffsetMinutes = 0
	}
	todayDate := s.nowFn().Add(time.Duration(tzOffsetMinutes) * time.Minute).Format("2006-01-02")
	records, err := s.repo.ListDailyRecords(ctx, userID, todayDate, todayDate)
	appSec := make(map[string]int64)
	domSec := make(map[string]int64)
	if err != nil {
		return appSec, domSec
	}
	for _, rec := range records {
		for k, a := range rec.Apps {
			appSec[strings.ToLower(k)] += a.Seconds
		}
		for k, d := range rec.Domains {
			domSec[NormalizeDomain(k)] += d.Seconds
		}
	}
	return appSec, domSec
}

func resolveEffectiveTzOffset(rec *WellbeingRulesRecord, explicitTz *int, explicitProvided bool) int {
	if explicitProvided && explicitTz != nil {
		tz := *explicitTz
		if tz >= -840 && tz <= 840 {
			return tz
		}
	}
	if rec != nil {
		if rec.Schedule.TimezoneOffsetMinutes != 0 {
			return rec.Schedule.TimezoneOffsetMinutes
		}
		if rec.LastTimezoneOffsetMinutes != nil {
			return *rec.LastTimezoneOffsetMinutes
		}
	}
	return 0
}

func computeExceededMap(limits map[string]TimeLimitRule, appSec, domSec map[string]int64) map[string]bool {
	out := make(map[string]bool, len(limits))
	for _, r := range evaluateLimits(limits, appSec, domSec) {
		if r.Exceeded {
			out[r.ID] = true
		}
	}
	return out
}

func evaluateLimits(raw map[string]TimeLimitRule, appSec, domSec map[string]int64) []TimeLimitRule {
	limits := make([]TimeLimitRule, 0, len(raw))
	for _, r := range raw {
		var used int64
		if r.TargetType == "domain" {
			used = domSec[NormalizeDomain(r.Target)]
		} else {
			used = appSec[strings.ToLower(r.Target)]
		}
		r.UsedSecondsToday = used
		r.Exceeded = r.Enabled && r.DailyLimitMinutes > 0 && used >= int64(r.DailyLimitMinutes)*60
		limits = append(limits, r)
	}
	sort.Slice(limits, func(i, j int) bool {
		return limits[i].ID < limits[j].ID
	})
	return limits
}

func (s *Service) GetRules(ctx context.Context, userID string, tzOffsetMinutes int) (*RulesResponse, error) {
	return s.getRulesInternal(ctx, userID, &tzOffsetMinutes, tzOffsetMinutes != 0)
}

func (s *Service) getRulesInternal(ctx context.Context, userID string, explicitTz *int, explicitProvided bool) (*RulesResponse, error) {
	rulesRec, err := s.repo.GetRulesRecord(ctx, userID)
	if err != nil {
		return nil, err
	}

	effectiveOffset := resolveEffectiveTzOffset(rulesRec, explicitTz, explicitProvided)
	if explicitProvided && explicitTz != nil && *explicitTz >= -840 && *explicitTz <= 840 {
		if rulesRec.LastTimezoneOffsetMinutes == nil || *rulesRec.LastTimezoneOffsetMinutes != effectiveOffset {
			unlockRules := s.lockUserRules(userID)
			if latest, gErr := s.repo.GetRulesRecord(ctx, userID); gErr == nil && latest != nil {
				tz := effectiveOffset
				latest.LastTimezoneOffsetMinutes = &tz
				_ = s.repo.SaveRulesRecord(ctx, *latest)
			}
			unlockRules()
		}
	}
	appSec, domSec := s.computeTodayUsageMaps(ctx, userID, effectiveOffset)
	limits := evaluateLimits(rulesRec.Limits, appSec, domSec)

	sched := rulesRec.Schedule
	if sched.Domains == nil {
		sched.Domains = []string{}
	}
	if sched.Apps == nil {
		sched.Apps = []string{}
	}
	localNow := s.nowFn().Add(time.Duration(effectiveOffset) * time.Minute)
	sched.ActiveNow = IsScheduleActiveAt(sched, localNow)

	return &RulesResponse{
		Limits:   limits,
		Schedule: sched,
	}, nil
}

func (s *Service) UpsertLimit(ctx context.Context, userID string, req UpsertLimitRequest) (*TimeLimitRule, error) {
	tType := strings.ToLower(strings.TrimSpace(req.TargetType))
	if tType != "domain" && tType != "app" {
		return nil, fmt.Errorf("%w: targetType must be domain or app", ErrInvalidLimit)
	}
	var target string
	if tType == "domain" {
		target = NormalizeDomain(req.Target)
		if isProtectedWellbeingDomain(target) || !strings.Contains(target, ".") || len(target) > 253 {
			return nil, fmt.Errorf("%w: invalid or protected domain target", ErrInvalidLimit)
		}
	} else {
		rawApp := strings.TrimSpace(req.Target)
		if rawApp == "" || len(rawApp) > 255 || strings.ContainsAny(rawApp, "/\\&|;><$`*?~!()[]{}\"' \n\r\t") {
			return nil, fmt.Errorf("%w: invalid app target characters", ErrInvalidLimit)
		}
		target = strings.ToLower(rawApp)
		if goals.IsProtectedAppTarget(target) || !desktopAppPattern.MatchString(target) {
			return nil, fmt.Errorf("%w: protected or invalid app target", ErrInvalidLimit)
		}
	}
	if req.DailyLimitMinutes < 1 || req.DailyLimitMinutes > 1440 {
		return nil, fmt.Errorf("%w: dailyLimitMinutes must be between 1 and 1440", ErrInvalidLimit)
	}
	disp := strings.TrimSpace(req.DisplayName)
	if disp == "" {
		disp = target
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	unlockRules := s.lockUserRules(userID)
	defer unlockRules()

	rulesRec, err := s.repo.GetRulesRecord(ctx, userID)
	if err != nil {
		return nil, err
	}
	ruleID := formatRuleID(tType, target)
	rule := TimeLimitRule{
		ID:                ruleID,
		TargetType:        tType,
		Target:            target,
		DisplayName:       disp,
		DailyLimitMinutes: req.DailyLimitMinutes,
		Enabled:           enabled,
	}
	if rulesRec.Limits == nil {
		rulesRec.Limits = make(map[string]TimeLimitRule)
	}
	rulesRec.Limits[ruleID] = rule
	rulesRec.UpdatedAt = s.nowFn().Format(time.RFC3339)

	if err := s.repo.SaveRulesRecord(ctx, *rulesRec); err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *Service) DeleteLimit(ctx context.Context, userID, ruleID string) error {
	cleanID := strings.TrimSpace(ruleID)
	if cleanID == "" {
		return ErrInvalidLimit
	}
	unlockRules := s.lockUserRules(userID)
	defer unlockRules()

	rulesRec, err := s.repo.GetRulesRecord(ctx, userID)
	if err != nil {
		return err
	}
	if _, exists := rulesRec.Limits[cleanID]; !exists {
		return fmt.Errorf("%w: rule not found", ErrInvalidLimit)
	}
	delete(rulesRec.Limits, cleanID)
	rulesRec.UpdatedAt = s.nowFn().Format(time.RFC3339)
	return s.repo.SaveRulesRecord(ctx, *rulesRec)
}

func (s *Service) SaveSchedule(ctx context.Context, userID string, cfg FocusScheduleConfig) (*FocusScheduleConfig, error) {
	if _, ok := parseHHMMMinutes(cfg.StartTime); !ok {
		return nil, ErrInvalidSchedule
	}
	if _, ok := parseHHMMMinutes(cfg.EndTime); !ok {
		return nil, ErrInvalidSchedule
	}
	if cfg.TimezoneOffsetMinutes < -840 || cfg.TimezoneOffsetMinutes > 840 {
		cfg.TimezoneOffsetMinutes = 0
	}
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = "Wind-Down Focus Hours"
	}
	cleanDomains := make([]string, 0, len(cfg.Domains))
	for _, d := range cfg.Domains {
		nd := NormalizeDomain(d)
		if nd != "" && !isProtectedWellbeingDomain(nd) && strings.Contains(nd, ".") {
			cleanDomains = append(cleanDomains, nd)
		}
	}
	cleanApps := make([]string, 0, len(cfg.Apps))
	for _, a := range cfg.Apps {
		na := strings.ToLower(strings.TrimSpace(a))
		if na != "" && !goals.IsProtectedAppTarget(na) && desktopAppPattern.MatchString(na) {
			cleanApps = append(cleanApps, na)
		}
	}

	unlockRules := s.lockUserRules(userID)
	defer unlockRules()

	rulesRec, err := s.repo.GetRulesRecord(ctx, userID)
	if err != nil {
		return nil, err
	}
	tzCopy := cfg.TimezoneOffsetMinutes
	rulesRec.LastTimezoneOffsetMinutes = &tzCopy
	cfg.Name = name
	cfg.Domains = cleanDomains
	cfg.Apps = cleanApps
	rulesRec.Schedule = cfg
	rulesRec.UpdatedAt = s.nowFn().Format(time.RFC3339)
	if err := s.repo.SaveRulesRecord(ctx, *rulesRec); err != nil {
		return nil, err
	}

	localNow := s.nowFn().Add(time.Duration(cfg.TimezoneOffsetMinutes) * time.Minute)
	cfg.ActiveNow = IsScheduleActiveAt(cfg, localNow)
	return &cfg, nil
}

// GetActiveWellbeingTargets converts exceeded daily limits and active Focus Schedules into
// synthetic enforcement Goal objects so Android, Browser Extension, and Desktop Agent enforce them uniformly.
func (s *Service) GetActiveWellbeingTargets(ctx context.Context, userID string) ([]goals.Goal, error) {
	rulesResp, err := s.getRulesInternal(ctx, userID, nil, false)
	if err != nil || rulesResp == nil {
		return nil, err
	}

	var activeGoals []goals.Goal

	// 1. Exceeded Daily Time Limits
	var exceededDomains []string
	var exceededApps []string
	for _, limit := range rulesResp.Limits {
		if !limit.Exceeded {
			continue
		}
		if limit.TargetType == "domain" {
			exceededDomains = append(exceededDomains, limit.Target)
		} else {
			exceededApps = append(exceededApps, limit.Target)
		}
	}
	if len(exceededDomains) > 0 || len(exceededApps) > 0 {
		activeGoals = append(activeGoals, goals.Goal{
			ID:                 "wellbeing-daily-limits",
			OwnerID:            userID,
			Title:              "Daily Time Limit Reached",
			Status:             "active",
			CreatedAt:          s.nowFn().Format(time.RFC3339),
			BlockedApps:        exceededApps,
			BlockedDomains:     exceededDomains,
			ApplicationTargets: appendAppTargets(nil, exceededApps),
		})
	}

	// 2. Active Focus Schedule / Wind-Down
	if rulesResp.Schedule.ActiveNow && (len(rulesResp.Schedule.Domains) > 0 || len(rulesResp.Schedule.Apps) > 0) {
		activeGoals = append(activeGoals, goals.Goal{
			ID:                 "wellbeing-focus-schedule",
			OwnerID:            userID,
			Title:              rulesResp.Schedule.Name,
			Status:             "active",
			CreatedAt:          s.nowFn().Format(time.RFC3339),
			BlockedApps:        rulesResp.Schedule.Apps,
			BlockedDomains:     rulesResp.Schedule.Domains,
			ApplicationTargets: appendAppTargets(nil, rulesResp.Schedule.Apps),
		})
	}

	return activeGoals, nil
}
