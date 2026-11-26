package wellbeing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type AppUsage struct {
	AppID       string `json:"appId"`
	DisplayName string `json:"displayName"`
	Seconds     int64  `json:"seconds"`
	Opens       int64  `json:"opens"`
}

type DomainUsage struct {
	Domain          string `json:"domain"`
	Seconds         int64  `json:"seconds"`
	Opens           int64  `json:"opens"`
	BlockedAttempts int64  `json:"blockedAttempts"`
}

type IngestPayload struct {
	Date                  string        `json:"date"`
	DeviceID              string        `json:"deviceId"`
	ClientType            string        `json:"clientType"`
	Platform              string        `json:"platform"`
	TotalScreenSeconds    int64         `json:"totalScreenSeconds"`
	Apps                  []AppUsage    `json:"apps"`
	Domains               []DomainUsage `json:"domains"`
	TimezoneOffsetMinutes *int          `json:"tzOffsetMinutes,omitempty"`
	SentAt                string        `json:"sentAt"`
}

var excludedSystemApps = map[string]bool{
	"lockapp.exe":             true,
	"logonui.exe":             true,
	"searchhost.exe":          true,
	"shellexperiencehost.exe": true,
	"startmenuexperiencehost.exe": true,
	"idle":                    true,
}

type dailyStore struct {
	Days map[string]map[string]AppUsage `json:"days"` // date -> appId -> AppUsage
}

type Tracker struct {
	mu              sync.Mutex
	storePath       string
	data            dailyStore
	lastAppID       string
	lastSampleAt    time.Time
	carryDuration   time.Duration
	lastSavedSec    int64
}

func NewTracker(dataDir string) *Tracker {
	t := &Tracker{
		data: dailyStore{Days: make(map[string]map[string]AppUsage)},
	}
	if dataDir != "" {
		t.storePath = filepath.Join(dataDir, "wellbeing_daily.json")
		t.load()
	}
	return t
}

func (t *Tracker) load() {
	if t.storePath == "" {
		return
	}
	raw, err := os.ReadFile(t.storePath)
	if err != nil {
		return
	}
	var parsed dailyStore
	if err := json.Unmarshal(raw, &parsed); err == nil && parsed.Days != nil {
		t.data = parsed
	}
}

func (t *Tracker) saveLocked() {
	if t.storePath == "" {
		return
	}
	t.pruneLocked()
	raw, err := json.MarshalIndent(t.data, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(t.storePath)
	_ = os.MkdirAll(dir, 0755)
	tmpPath := t.storePath + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0600); err == nil {
		_ = os.Rename(tmpPath, t.storePath)
	} else {
		_ = os.WriteFile(t.storePath, raw, 0600)
	}
}

func (t *Tracker) pruneLocked() {
	if len(t.data.Days) <= 14 {
		return
	}
	keys := make([]string, 0, len(t.data.Days))
	for k := range t.data.Days {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i := 0; i < len(keys)-14; i++ {
		delete(t.data.Days, keys[i])
	}
}

func FormatDisplayName(exeName string) string {
	clean := strings.TrimSpace(exeName)
	lower := strings.ToLower(clean)
	lower = strings.TrimSuffix(lower, ".exe")
	switch lower {
	case "code":
		return "VS Code"
	case "chrome":
		return "Google Chrome"
	case "msedge":
		return "Microsoft Edge"
	case "brave":
		return "Brave Browser"
	case "firefox":
		return "Mozilla Firefox"
	case "discord":
		return "Discord"
	case "spotify":
		return "Spotify"
	case "slack":
		return "Slack"
	case "windowsterminal", "wt":
		return "Windows Terminal"
	case "explorer":
		return "File Explorer"
	default:
		if clean == "" {
			return "Unknown"
		}
		base := strings.TrimSuffix(clean, ".exe")
		base = strings.TrimSuffix(base, ".EXE")
		if len(base) > 1 {
			return strings.ToUpper(base[:1]) + base[1:]
		}
		return base
	}
}

// RecordSample advances daily foreground screen time & launch counts while user is active (not idle).
func (t *Tracker) RecordSample(appID string, elapsedSec int64, isIdle bool, now time.Time) {
	if elapsedSec < 0 {
		elapsedSec = 0
	}
	t.RecordSampleDuration(appID, time.Duration(elapsedSec)*time.Second, isIdle, now)
}

// RecordSampleDuration accumulates sub-second durations without truncating ticks and splits across midnight boundaries.
func (t *Tracker) RecordSampleDuration(appID string, elapsed time.Duration, isIdle bool, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if isIdle || appID == "" {
		t.lastAppID = ""
		t.carryDuration = 0
		t.lastSampleAt = now
		return
	}

	normID := strings.ToLower(strings.TrimSpace(appID))
	if excludedSystemApps[normID] {
		return
	}

	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > 300*time.Second {
		elapsed = 300 * time.Second // Cap per-tick delta cleanly across sleep/suspend
	}

	isSwitch := normID != t.lastAppID
	if isSwitch {
		// Commit any prior carry cleanly for the new app timer window
		t.carryDuration = 0
	}

	totalDur := t.carryDuration + elapsed
	elapsedSec := int64(totalDur / time.Second)
	t.carryDuration = totalDur % time.Second

	localNow := now.Local()
	endSec := elapsedSec
	startLocal := localNow.Add(-time.Duration(endSec) * time.Second)

	remaining := endSec
	cursor := startLocal
	var totalWrittenSec int64
	openedRecorded := false

	for remaining > 0 || (isSwitch && !openedRecorded) {
		dateKey := cursor.Format("2006-01-02")
		y, m, d := cursor.Date()
		nextMidnight := time.Date(y, m, d+1, 0, 0, 0, 0, cursor.Location())
		secUntilMidnight := int64(nextMidnight.Sub(cursor).Seconds())
		if secUntilMidnight <= 0 {
			secUntilMidnight = remaining
		}
		sliceSec := remaining
		if secUntilMidnight < sliceSec {
			sliceSec = secUntilMidnight
		}

		if t.data.Days[dateKey] == nil {
			t.data.Days[dateKey] = make(map[string]AppUsage)
		}
		entry := t.data.Days[dateKey][normID]
		entry.AppID = normID
		entry.DisplayName = FormatDisplayName(appID)
		entry.Seconds += sliceSec
		if isSwitch && !openedRecorded {
			entry.Opens++
			openedRecorded = true
		}
		t.data.Days[dateKey][normID] = entry
		totalWrittenSec += sliceSec

		remaining -= sliceSec
		cursor = cursor.Add(time.Duration(sliceSec) * time.Second)
		if remaining <= 0 {
			break
		}
	}

	if isSwitch {
		t.lastAppID = normID
	}
	t.lastSampleAt = now
	t.lastSavedSec += totalWrittenSec

	// Persist roughly every 10 seconds or on app switch to minimize disk churn
	if isSwitch || t.lastSavedSec >= 10 {
		t.lastSavedSec = 0
		t.saveLocked()
	}
}

func (t *Tracker) BuildIngestPayload(deviceID, platform string, now time.Time) IngestPayload {
	return t.BuildIngestPayloadForDate(deviceID, platform, now.Local().Format("2006-01-02"), now)
}

func (t *Tracker) BuildIngestPayloadForDate(deviceID, platform, dateKey string, now time.Time) IngestPayload {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.saveLocked()
	dayApps := t.data.Days[dateKey]

	apps := make([]AppUsage, 0, len(dayApps))
	var totalSec int64
	for _, a := range dayApps {
		apps = append(apps, a)
		totalSec += a.Seconds
	}
	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Seconds > apps[j].Seconds
	})

	_, offsetSec := now.Local().Zone()
	tzMin := offsetSec / 60

	return IngestPayload{
		Date:                  dateKey,
		DeviceID:              deviceID,
		ClientType:            "desktop_agent",
		Platform:              platform,
		TotalScreenSeconds:    totalSec,
		Apps:                  apps,
		Domains:               []DomainUsage{},
		TimezoneOffsetMinutes: &tzMin,
		SentAt:                now.UTC().Format(time.RFC3339),
	}
}
