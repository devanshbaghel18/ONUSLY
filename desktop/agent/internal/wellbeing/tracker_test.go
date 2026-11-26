package wellbeing

import (
	"testing"
	"time"
)

func TestTracker_TracksDurationOpensAndIdle(t *testing.T) {
	tmp := t.TempDir()
	tr := NewTracker(tmp)

	base := time.Date(2026, 11, 26, 10, 0, 0, 0, time.Local)
	tr.RecordSample("Code.exe", 10, false, base)
	tr.RecordSample("Code.exe", 20, false, base.Add(20*time.Second))
	tr.RecordSample("chrome.exe", 15, false, base.Add(35*time.Second))
	tr.RecordSample("", 60, true, base.Add(95*time.Second))
	tr.RecordSample("Code.exe", 5, false, base.Add(100*time.Second))

	payload := tr.BuildIngestPayload("win-1", "windows", base.Add(100*time.Second))
	if payload.TotalScreenSeconds != 50 {
		t.Fatalf("TotalScreenSeconds = %d, want 50", payload.TotalScreenSeconds)
	}
	if len(payload.Apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(payload.Apps))
	}

	var code, chrome AppUsage
	for _, a := range payload.Apps {
		if a.AppID == "code.exe" {
			code = a
		} else if a.AppID == "chrome.exe" {
			chrome = a
		}
	}
	if code.Seconds != 35 || code.Opens != 2 || code.DisplayName != "VS Code" {
		t.Fatalf("unexpected code usage: %+v", code)
	}
	if chrome.Seconds != 15 || chrome.Opens != 1 || chrome.DisplayName != "Google Chrome" {
		t.Fatalf("unexpected chrome usage: %+v", chrome)
	}
}
