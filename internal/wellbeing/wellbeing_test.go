package wellbeing

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

func newTestService(fixedNow time.Time) *Service {
	repo := NewDynamoRepository(nil, "Onusly-Test")
	svc := NewService(repo)
	svc.nowFn = func() time.Time { return fixedNow }
	return svc
}

func TestNormalizeDomain_PrivacyAndDeduplication(t *testing.T) {
	cases := map[string]string{
		"https://www.Instagram.com/reels/123?foo=bar": "instagram.com",
		"WWW.YouTube.com:443/watch?v=abc":             "youtube.com",
		"m.reddit.com.":                               "m.reddit.com",
		"  ":                                          "",
	}
	for in, want := range cases {
		if got := NormalizeDomain(in); got != want {
			t.Fatalf("NormalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIngest_IdempotentMonotonicAndAllTimeDeltas(t *testing.T) {
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc := newTestService(now)
	ctx := context.Background()

	// 1. First snapshot for Today (extension)
	_, err := svc.Ingest(ctx, "user-1", IngestRequest{
		Date:               "2026-11-26",
		DeviceID:           "ext-chrome-1",
		ClientType:         "extension",
		Platform:           "chrome",
		TotalScreenSeconds: 600,
		Domains: []DomainUsageEntry{
			{Domain: "https://www.instagram.com/feed", Seconds: 400, Opens: 5, BlockedAttempts: 1},
			{Domain: "github.com", Seconds: 200, Opens: 3, BlockedAttempts: 0},
		},
	})
	if err != nil {
		t.Fatalf("Ingest 1 failed: %v", err)
	}

	// 2. Duplicate retry (must NOT double count!)
	_, err = svc.Ingest(ctx, "user-1", IngestRequest{
		Date:               "2026-11-26",
		DeviceID:           "ext-chrome-1",
		ClientType:         "extension",
		Platform:           "chrome",
		TotalScreenSeconds: 600,
		Domains: []DomainUsageEntry{
			{Domain: "instagram.com", Seconds: 400, Opens: 5, BlockedAttempts: 1},
			{Domain: "github.com", Seconds: 200, Opens: 3, BlockedAttempts: 0},
		},
	})
	if err != nil {
		t.Fatalf("Ingest retry failed: %v", err)
	}

	// 3. Advancing update for same day (+120s on instagram, +2 opens)
	_, err = svc.Ingest(ctx, "user-1", IngestRequest{
		Date:               "2026-11-26",
		DeviceID:           "ext-chrome-1",
		ClientType:         "extension",
		Platform:           "chrome",
		TotalScreenSeconds: 720,
		Domains: []DomainUsageEntry{
			{Domain: "instagram.com", Seconds: 520, Opens: 7, BlockedAttempts: 2},
			{Domain: "github.com", Seconds: 200, Opens: 3, BlockedAttempts: 0},
		},
	})
	if err != nil {
		t.Fatalf("Ingest 3 failed: %v", err)
	}

	// 4. Add Android usage for Yesterday
	_, err = svc.Ingest(ctx, "user-1", IngestRequest{
		Date:               "2026-11-25",
		DeviceID:           "android-pixel-1",
		ClientType:         "android",
		Platform:           "android",
		TotalScreenSeconds: 1800,
		Apps: []AppUsageEntry{
			{AppID: "com.instagram.android", DisplayName: "Instagram", Seconds: 900, Opens: 12},
			{AppID: "com.whatsapp", DisplayName: "WhatsApp", Seconds: 900, Opens: 20},
		},
	})
	if err != nil {
		t.Fatalf("Ingest 4 failed: %v", err)
	}

	// Today summary
	todaySum, err := svc.GetSummary(ctx, "user-1", "today", 0, "")
	if err != nil {
		t.Fatalf("GetSummary today failed: %v", err)
	}
	if todaySum.TotalScreenSeconds != 720 {
		t.Fatalf("today TotalScreenSeconds = %d, want 720", todaySum.TotalScreenSeconds)
	}
	if todaySum.TotalWebsiteOpens != 10 {
		t.Fatalf("today TotalWebsiteOpens = %d, want 10", todaySum.TotalWebsiteOpens)
	}
	if todaySum.TotalBlockedAttempts != 2 {
		t.Fatalf("today TotalBlockedAttempts = %d, want 2", todaySum.TotalBlockedAttempts)
	}
	// Comparison delta: Today (720) - Yesterday (1800) = -1080
	if todaySum.ComparisonDeltaSeconds != -1080 {
		t.Fatalf("today ComparisonDeltaSeconds = %d, want -1080", todaySum.ComparisonDeltaSeconds)
	}
	if len(todaySum.DailySeries) != 7 {
		t.Fatalf("expected 7 daily points, got %d", len(todaySum.DailySeries))
	}

	// Yesterday summary
	yestSum, err := svc.GetSummary(ctx, "user-1", "yesterday", 0, "")
	if err != nil {
		t.Fatalf("GetSummary yesterday failed: %v", err)
	}
	if yestSum.TotalScreenSeconds != 1800 || len(yestSum.TopApps) != 2 {
		t.Fatalf("unexpected yesterday summary: %+v", yestSum)
	}

	// 7-day summary
	weekSum, err := svc.GetSummary(ctx, "user-1", "7d", 0, "")
	if err != nil {
		t.Fatalf("GetSummary 7d failed: %v", err)
	}
	if weekSum.TotalScreenSeconds != 2520 {
		t.Fatalf("7d TotalScreenSeconds = %d, want 2520", weekSum.TotalScreenSeconds)
	}

	// All-time summary
	allSum, err := svc.GetSummary(ctx, "user-1", "all", 0, "")
	if err != nil {
		t.Fatalf("GetSummary all failed: %v", err)
	}
	if allSum.TotalScreenSeconds != 2520 {
		t.Fatalf("all TotalScreenSeconds = %d, want 2520", allSum.TotalScreenSeconds)
	}
	if allSum.TotalWebsiteOpens != 10 {
		t.Fatalf("all TotalWebsiteOpens = %d, want 10", allSum.TotalWebsiteOpens)
	}

	// User isolation check: user-2 sees 0
	otherSum, err := svc.GetSummary(ctx, "user-2", "all", 0, "")
	if err != nil {
		t.Fatalf("GetSummary user-2 failed: %v", err)
	}
	if otherSum.TotalScreenSeconds != 0 || len(otherSum.TopApps) != 0 {
		t.Fatalf("cross-user data leak detected: %+v", otherSum)
	}
}

func TestHandler_HTTPIngestAndSummary(t *testing.T) {
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc := newTestService(now)
	h := NewHandler(svc)

	body, _ := json.Marshal(IngestRequest{
		Date:               "2026-11-26",
		DeviceID:           "win-1",
		ClientType:         "desktop_agent",
		Platform:           "windows",
		TotalScreenSeconds: 300,
		Apps: []AppUsageEntry{
			{AppID: "code.exe", DisplayName: "VS Code", Seconds: 300, Opens: 2},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/wellbeing/ingest", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "u-http"))
	rec := httptest.NewRecorder()
	h.Ingest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Ingest HTTP code = %d, body = %s", rec.Code, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/wellbeing/summary?range=today&tzOffset=0", nil)
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), middleware.UserIDKey, "u-http"))
	getRec := httptest.NewRecorder()
	h.GetSummary(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetSummary HTTP code = %d, body = %s", getRec.Code, getRec.Body.String())
	}

	var parsed SummaryResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("failed decoding summary: %v", err)
	}
	if parsed.TotalScreenSeconds != 300 || len(parsed.TopApps) != 1 || parsed.TopApps[0].DisplayName != "VS Code" {
		t.Fatalf("unexpected summary payload: %+v", parsed)
	}
}

func TestDailyLimitsAndFocusSchedule_TriggerActiveTargets(t *testing.T) {
	now := time.Date(2026, 11, 26, 23, 30, 0, 0, time.UTC)
	svc := newTestService(now)
	ctx := context.Background()

	// Configure a 10m daily limit for instagram.com
	enabled := true
	rule, err := svc.UpsertLimit(ctx, "user-lim", UpsertLimitRequest{
		TargetType:        "domain",
		Target:            "https://www.instagram.com/",
		DisplayName:       "Instagram",
		DailyLimitMinutes: 10,
		Enabled:           &enabled,
	})
	if err != nil {
		t.Fatalf("UpsertLimit failed: %v", err)
	}
	if rule.ID != "domain:instagram.com" {
		t.Fatalf("unexpected rule ID: %s", rule.ID)
	}

	// Ingest 6m usage (360s < 600s) -> not exceeded yet
	_, err = svc.Ingest(ctx, "user-lim", IngestRequest{
		Date:       "2026-11-26",
		DeviceID:   "ext-1",
		ClientType: "extension",
		Platform:   "chrome",
		Domains: []DomainUsageEntry{
			{Domain: "instagram.com", Seconds: 360, Opens: 4},
		},
	})
	if err != nil {
		t.Fatalf("Ingest 1 failed: %v", err)
	}

	targetsBefore, err := svc.GetActiveWellbeingTargets(ctx, "user-lim")
	if err != nil {
		t.Fatalf("GetActiveWellbeingTargets failed: %v", err)
	}
	if len(targetsBefore) != 0 {
		t.Fatalf("expected 0 targets before quota exceeded, got %d", len(targetsBefore))
	}

	// Ingest 11m usage (660s >= 600s) + overnight focus schedule (23:00-07:00)
	_, err = svc.Ingest(ctx, "user-lim", IngestRequest{
		Date:       "2026-11-26",
		DeviceID:   "ext-1",
		ClientType: "extension",
		Platform:   "chrome",
		Domains: []DomainUsageEntry{
			{Domain: "instagram.com", Seconds: 660, Opens: 8},
		},
	})
	if err != nil {
		t.Fatalf("Ingest 2 failed: %v", err)
	}

	_, err = svc.SaveSchedule(ctx, "user-lim", FocusScheduleConfig{
		Enabled:   true,
		Name:      "Bedtime Wind-Down",
		StartTime: "23:00",
		EndTime:   "07:00",
		Domains:   []string{"youtube.com"},
		Apps:      []string{"com.google.android.youtube"},
	})
	if err != nil {
		t.Fatalf("SaveSchedule failed: %v", err)
	}

	targetsAfter, err := svc.GetActiveWellbeingTargets(ctx, "user-lim")
	if err != nil {
		t.Fatalf("GetActiveWellbeingTargets after failed: %v", err)
	}
	if len(targetsAfter) != 2 {
		t.Fatalf("expected 2 targets (exceeded limit + active overnight schedule), got %d: %+v", len(targetsAfter), targetsAfter)
	}

	// Deleting limit removes it cleanly
	if err := svc.DeleteLimit(ctx, "user-lim", "domain:instagram.com"); err != nil {
		t.Fatalf("DeleteLimit failed: %v", err)
	}
	rulesAfter, err := svc.GetRules(ctx, "user-lim", 0)
	if err != nil || len(rulesAfter.Limits) != 0 {
		t.Fatalf("expected 0 limits after delete, got %+v", rulesAfter)
	}
}

type captureLimitNotifier struct {
	events [][]TimeLimitRule
}

func (c *captureLimitNotifier) NotifyLimitReached(_ context.Context, _ string, exceeded []TimeLimitRule) {
	c.events = append(c.events, exceeded)
}

func TestWellbeing_LimitNotifierProtectedTargetsAndAppClassification(t *testing.T) {
	now := time.Date(2026, 11, 26, 20, 0, 0, 0, time.UTC)
	svc := newTestService(now)
	notifier := &captureLimitNotifier{}
	svc.SetLimitNotifier(notifier)
	ctx := context.Background()

	// Protected domain & protected system process must be rejected
	if _, err := svc.UpsertLimit(ctx, "u-sec", UpsertLimitRequest{
		TargetType: "domain", Target: "onusly.com", DailyLimitMinutes: 5,
	}); err == nil {
		t.Fatalf("expected error for protected domain onusly.com")
	}
	if _, err := svc.UpsertLimit(ctx, "u-sec", UpsertLimitRequest{
		TargetType: "app", Target: "explorer.exe", DailyLimitMinutes: 5,
	}); err == nil {
		t.Fatalf("expected error for protected system process explorer.exe")
	}

	// Extensionless desktop app must be classified as desktop_app, not android_package
	enabled := true
	if _, err := svc.UpsertLimit(ctx, "u-sec", UpsertLimitRequest{
		TargetType: "app", Target: "discord", DisplayName: "Discord", DailyLimitMinutes: 5, Enabled: &enabled,
	}); err != nil {
		t.Fatalf("UpsertLimit desktop app failed: %v", err)
	}
	if _, err := svc.UpsertLimit(ctx, "u-sec", UpsertLimitRequest{
		TargetType: "app", Target: "com.instagram.android", DisplayName: "Instagram", DailyLimitMinutes: 5, Enabled: &enabled,
	}); err != nil {
		t.Fatalf("UpsertLimit android app failed: %v", err)
	}

	// IST timezone (+330) shifts 20:00 UTC on Nov 26 to 01:30 Nov 27
	tz := 330
	if _, err := svc.Ingest(ctx, "u-sec", IngestRequest{
		Date: "2026-11-27", DeviceID: "win-1", ClientType: "desktop_agent", Platform: "windows",
		TimezoneOffsetMinutes: &tz,
		Apps:                  []AppUsageEntry{{AppID: "discord", Seconds: 320, Opens: 3}},
	}); err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	if len(notifier.events) != 1 || len(notifier.events[0]) != 1 || notifier.events[0][0].Target != "discord" {
		t.Fatalf("expected 1 limit reached event for discord, got %+v", notifier.events)
	}

	// Second ingest above threshold must NOT duplicate limit_reached push
	if _, err := svc.Ingest(ctx, "u-sec", IngestRequest{
		Date: "2026-11-27", DeviceID: "win-1", ClientType: "desktop_agent", Platform: "windows",
		Apps: []AppUsageEntry{{AppID: "discord", Seconds: 450, Opens: 4}},
	}); err != nil {
		t.Fatalf("Ingest 2 failed: %v", err)
	}
	if len(notifier.events) != 1 {
		t.Fatalf("expected no duplicate limit reached event, got %d", len(notifier.events))
	}

	// GetActiveWellbeingTargets should use remembered tz (+330 -> Nov 27) and mark discord as desktop_app
	targets, err := svc.GetActiveWellbeingTargets(ctx, "u-sec")
	if err != nil || len(targets) != 1 || len(targets[0].ApplicationTargets) != 1 {
		t.Fatalf("expected 1 active desktop_app target via remembered timezone, got %+v (err=%v)", targets, err)
	}
	if targets[0].ApplicationTargets[0].Kind != "desktop_app" || targets[0].ApplicationTargets[0].Identifier != "discord" {
		t.Fatalf("unexpected application target classification: %+v", targets[0].ApplicationTargets[0])
	}
}
