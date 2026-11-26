package wellbeing

import "errors"

var (
	ErrInvalidDate       = errors.New("invalid date: expected YYYY-MM-DD")
	ErrInvalidDeviceID   = errors.New("invalid deviceId: must not be empty")
	ErrInvalidClientType = errors.New("invalid clientType: must be android, extension, desktop_agent, or web")
	ErrInvalidRange      = errors.New("invalid range: must be today, yesterday, 7d, or all")
	ErrInvalidLimit      = errors.New("invalid daily limit rule")
	ErrInvalidSchedule   = errors.New("invalid focus schedule (expected HH:MM 24h times)")
)

const MaxSecondsPerDay = 86400

// AppUsageEntry represents screen time and launch counts for a single application.
type AppUsageEntry struct {
	AppID       string `dynamodbav:"AppID" json:"appId"`
	DisplayName string `dynamodbav:"DisplayName" json:"displayName"`
	Seconds     int64  `dynamodbav:"Seconds" json:"seconds"`
	Opens       int64  `dynamodbav:"Opens" json:"opens"`
}

// DomainUsageEntry represents browsing duration, visit count, and blocked attempts for a hostname.
type DomainUsageEntry struct {
	Domain          string `dynamodbav:"Domain" json:"domain"`
	Seconds         int64  `dynamodbav:"Seconds" json:"seconds"`
	Opens           int64  `dynamodbav:"Opens" json:"opens"`
	BlockedAttempts int64  `dynamodbav:"BlockedAttempts" json:"blockedAttempts"`
}

// DailyDeviceRecord persists cumulative daily wellbeing counters for one (User, Date, Device) tuple.
type DailyDeviceRecord struct {
	PK                 string                      `dynamodbav:"PK" json:"-"`
	SK                 string                      `dynamodbav:"SK" json:"-"`
	UserID             string                      `dynamodbav:"UserID" json:"userId"`
	Date               string                      `dynamodbav:"Date" json:"date"`
	DeviceID           string                      `dynamodbav:"DeviceID" json:"deviceId"`
	ClientType         string                      `dynamodbav:"ClientType" json:"clientType"`
	Platform           string                      `dynamodbav:"Platform" json:"platform"`
	TotalScreenSeconds int64                       `dynamodbav:"TotalScreenSeconds" json:"totalScreenSeconds"`
	Apps               map[string]AppUsageEntry    `dynamodbav:"Apps" json:"apps"`
	Domains            map[string]DomainUsageEntry `dynamodbav:"Domains" json:"domains"`
	UpdatedAt          string                      `dynamodbav:"UpdatedAt" json:"updatedAt"`
}

// AllTimeRecord stores aggregated lifetime totals per user across all days and devices.
type AllTimeRecord struct {
	PK                 string                      `dynamodbav:"PK" json:"-"`
	SK                 string                      `dynamodbav:"SK" json:"-"`
	UserID             string                      `dynamodbav:"UserID" json:"userId"`
	TotalScreenSeconds int64                       `dynamodbav:"TotalScreenSeconds" json:"totalScreenSeconds"`
	Apps               map[string]AppUsageEntry    `dynamodbav:"Apps" json:"apps"`
	Domains            map[string]DomainUsageEntry `dynamodbav:"Domains" json:"domains"`
	UpdatedAt          string                      `dynamodbav:"UpdatedAt" json:"updatedAt"`
}

// IngestRequest is submitted by Android, Extension, and Desktop clients with daily cumulative usage.
type IngestRequest struct {
	Date                  string             `json:"date"`
	DeviceID              string             `json:"deviceId"`
	ClientType            string             `json:"clientType"`
	Platform              string             `json:"platform"`
	TotalScreenSeconds    int64              `json:"totalScreenSeconds"`
	Apps                  []AppUsageEntry    `json:"apps"`
	Domains               []DomainUsageEntry `json:"domains"`
	TimezoneOffsetMinutes *int               `json:"tzOffsetMinutes,omitempty"`
	SentAt                string             `json:"sentAt"`
}

// IngestResponse acknowledges processed telemetry.
type IngestResponse struct {
	Status             string `json:"status"`
	Date               string `json:"date"`
	DeviceID           string `json:"deviceId"`
	TotalScreenSeconds int64  `json:"totalScreenSeconds"`
	AppsCount          int    `json:"appsCount"`
	DomainsCount       int    `json:"domainsCount"`
}

// DailyPoint summarizes one calendar day for trend charts.
type DailyPoint struct {
	Date            string `json:"date"`
	TotalSeconds    int64  `json:"totalSeconds"`
	AppSeconds      int64  `json:"appSeconds"`
	WebSeconds      int64  `json:"webSeconds"`
	WebsiteOpens    int64  `json:"websiteOpens"`
	BlockedAttempts int64  `json:"blockedAttempts"`
}

// AggregatedApp summarizes an app across devices in the requested range.
type AggregatedApp struct {
	AppID       string   `json:"appId"`
	DisplayName string   `json:"displayName"`
	Seconds     int64    `json:"seconds"`
	Opens       int64    `json:"opens"`
	Platforms   []string `json:"platforms"`
}

// AggregatedWebsite summarizes a website across devices in the requested range.
type AggregatedWebsite struct {
	Domain          string   `json:"domain"`
	Seconds         int64    `json:"seconds"`
	Opens           int64    `json:"opens"`
	BlockedAttempts int64    `json:"blockedAttempts"`
	Platforms       []string `json:"platforms"`
}

// DeviceSummary summarizes screen time contributed by a single device.
type DeviceSummary struct {
	DeviceID           string `json:"deviceId"`
	ClientType         string `json:"clientType"`
	Platform           string `json:"platform"`
	TotalScreenSeconds int64  `json:"totalScreenSeconds"`
	WebsiteOpens       int64  `json:"websiteOpens"`
	BlockedAttempts    int64  `json:"blockedAttempts"`
	LastUpdatedAt      string `json:"lastUpdatedAt"`
}

// SummaryResponse returns aggregated Digital Wellbeing analytics for today, yesterday, 7d, or all-time.
type SummaryResponse struct {
	Range                  string              `json:"range"`
	StartDate              string              `json:"startDate"`
	EndDate                string              `json:"endDate"`
	TotalScreenSeconds     int64               `json:"totalScreenSeconds"`
	TotalAppSeconds        int64               `json:"totalAppSeconds"`
	TotalWebSeconds        int64               `json:"totalWebSeconds"`
	ComparisonDeltaSeconds int64               `json:"comparisonDeltaSeconds"`
	TotalWebsiteOpens      int64               `json:"totalWebsiteOpens"`
	TotalBlockedAttempts   int64               `json:"totalBlockedAttempts"`
	DailySeries            []DailyPoint        `json:"dailySeries"`
	TopApps                []AggregatedApp     `json:"topApps"`
	TopWebsites            []AggregatedWebsite `json:"topWebsites"`
	Devices                []DeviceSummary     `json:"devices"`
}

// TimeLimitRule configures a daily quota for a website domain or app identifier.
type TimeLimitRule struct {
	ID                string `dynamodbav:"ID" json:"id"`
	TargetType        string `dynamodbav:"TargetType" json:"targetType"` // "domain" | "app"
	Target            string `dynamodbav:"Target" json:"target"`
	DisplayName       string `dynamodbav:"DisplayName" json:"displayName"`
	DailyLimitMinutes int    `dynamodbav:"DailyLimitMinutes" json:"dailyLimitMinutes"`
	Enabled           bool   `dynamodbav:"Enabled" json:"enabled"`
	UsedSecondsToday  int64  `dynamodbav:"-" json:"usedSecondsToday"`
	Exceeded          bool   `dynamodbav:"-" json:"exceeded"`
}

// FocusScheduleConfig represents recurring quiet hours / wind-down enforcement.
type FocusScheduleConfig struct {
	Enabled               bool     `dynamodbav:"Enabled" json:"enabled"`
	Name                  string   `dynamodbav:"Name" json:"name"`
	StartTime             string   `dynamodbav:"StartTime" json:"startTime"` // "23:00"
	EndTime               string   `dynamodbav:"EndTime" json:"endTime"`     // "07:00"
	TimezoneOffsetMinutes int      `dynamodbav:"TimezoneOffsetMinutes" json:"timezoneOffsetMinutes"`
	Domains               []string `dynamodbav:"Domains" json:"domains"`
	Apps                  []string `dynamodbav:"Apps" json:"apps"`
	ActiveNow             bool     `dynamodbav:"-" json:"activeNow"`
}

// WellbeingRulesRecord persists a user's daily time limits and focus schedule in DynamoDB.
type WellbeingRulesRecord struct {
	PK                        string                   `dynamodbav:"PK" json:"-"`
	SK                        string                   `dynamodbav:"SK" json:"-"`
	UserID                    string                   `dynamodbav:"UserID" json:"userId"`
	Limits                    map[string]TimeLimitRule `dynamodbav:"Limits" json:"limits"`
	Schedule                  FocusScheduleConfig      `dynamodbav:"Schedule" json:"schedule"`
	LastTimezoneOffsetMinutes *int                     `dynamodbav:"LastTimezoneOffsetMinutes,omitempty" json:"lastTimezoneOffsetMinutes,omitempty"`
	UpdatedAt                 string                   `dynamodbav:"UpdatedAt" json:"updatedAt"`
}

// UpsertLimitRequest creates or updates a daily quota rule.
type UpsertLimitRequest struct {
	TargetType        string `json:"targetType"`
	Target            string `json:"target"`
	DisplayName       string `json:"displayName"`
	DailyLimitMinutes int    `json:"dailyLimitMinutes"`
	Enabled           *bool  `json:"enabled"`
}

// RulesResponse returns configured daily limits (with live usage today) and focus schedule status.
type RulesResponse struct {
	Limits   []TimeLimitRule     `json:"limits"`
	Schedule FocusScheduleConfig `json:"schedule"`
}
