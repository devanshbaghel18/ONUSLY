package apps

import (
	"testing"
)

func TestIsProtectedSystemProcess(t *testing.T) {
	protected := []struct {
		path string
		base string
	}{
		{`C:\Windows\explorer.exe`, "explorer.exe"},
		{`C:\Windows\System32\csrss.exe`, "csrss.exe"},
		{`C:\Windows\System32\svchost.exe`, "svchost.exe"},
		{`C:\Program Files\Onusly\onusly-agent.exe`, "onusly-agent.exe"},
		{`C:\Program Files\Onusly\onusly-native-host.exe`, "onusly-native-host.exe"},
		{"/usr/lib/systemd/systemd", "systemd"},
		{"/bin/bash", "bash"},
		{"/usr/local/bin/onusly-agent", "onusly-agent"},
	}

	for _, tc := range protected {
		if !IsProtectedSystemProcess(tc.path, tc.base) {
			t.Errorf("Expected process %s (%s) to be protected system process", tc.path, tc.base)
		}
	}

	unprotected := []struct {
		path string
		base string
	}{
		{`C:\Users\User\AppData\Local\Discord\app-1.0.9001\Discord.exe`, "Discord.exe"},
		{`C:\Program Files (x86)\Steam\steam.exe`, "steam.exe"},
		{`C:\Games\customgame.exe`, "customgame.exe"},
		{"/usr/bin/spotify", "spotify"},
	}

	for _, tc := range unprotected {
		if IsProtectedSystemProcess(tc.path, tc.base) {
			t.Errorf("Expected process %s (%s) to NOT be protected", tc.path, tc.base)
		}
	}
}

func TestMatchProcess_PrecedenceAndEvasion(t *testing.T) {
	targets := []TargetApp{
		{
			GoalID:     "g1",
			AppID:      "discord",
			Restricted: true,
			Aliases:    []string{"discord-ptb"},
		},
		{
			GoalID:     "g2",
			AppID:      "steam",
			Restricted: true,
		},
		{
			GoalID:     "g3",
			AppID:      "explorer", // Malicious or accidental target trying to block explorer
			Restricted: true,
		},
	}

	// 1. Critical system process immunity check
	explorerProc := ProcessInfo{
		PID:      1000,
		ExePath:  `C:\Windows\explorer.exe`,
		BaseName: "explorer.exe",
	}
	_, _, _, matched := MatchProcess(explorerProc, targets)
	if matched {
		t.Fatalf("SECURITY VIOLATION: explorer.exe was matched against policy target!")
	}

	// 2. Exact match
	discordProc := ProcessInfo{
		PID:      2000,
		ExePath:  `C:\Users\test\AppData\Local\Discord\Discord.exe`,
		BaseName: "Discord.exe",
	}
	tgt, conf, reason, matched := MatchProcess(discordProc, targets)
	if !matched || tgt.AppID != "discord" {
		t.Fatalf("Expected Discord.exe to match target discord, got matched=%t, target=%v", matched, tgt)
	}
	if conf == ConfidenceNone {
		t.Errorf("Expected valid match confidence, got %v", conf)
	}
	if reason == "" {
		t.Errorf("Expected match reason to be populated")
	}

	// 3. Catalog match (steamwebhelper.exe -> steam)
	steamHelperProc := ProcessInfo{
		PID:      3000,
		ExePath:  `C:\Program Files (x86)\Steam\bin\steamwebhelper.exe`,
		BaseName: "steamwebhelper.exe",
	}
	tgtSteam, confSteam, _, matchedSteam := MatchProcess(steamHelperProc, targets)
	if !matchedSteam || tgtSteam.AppID != "steam" {
		t.Fatalf("Expected steamwebhelper.exe to match target steam, got matched=%t, target=%v", matchedSteam, tgtSteam)
	}
	if confSteam != ConfidenceDirBinary {
		t.Errorf("Expected ConfidenceDirBinary for catalog match, got %v", confSteam)
	}

	// 4. Strict boundary checks preventing substring false positives
	falsePositives := []ProcessInfo{
		{PID: 4001, ExePath: `C:\bin\notdiscord.exe`, BaseName: "notdiscord.exe"},
		{PID: 4002, ExePath: `C:\bin\discordant.exe`, BaseName: "discordant.exe"},
		{PID: 4003, ExePath: `C:\bin\steamengine.exe`, BaseName: "steamengine.exe"},
	}

	for _, fp := range falsePositives {
		_, _, _, m := MatchProcess(fp, targets)
		if m {
			t.Errorf("Boundary check failed: %s falsely matched a target!", fp.BaseName)
		}
	}

	// 5. Reverse-DNS identifiers must NOT automatically match an executable merely because their final component equals its basename
	reverseDnsTargets := []TargetApp{
		{
			GoalID:     "g-android-whatsapp",
			AppID:      "com.whatsapp",
			Restricted: true,
		},
		{
			GoalID:     "g-android-spotify",
			AppID:      "com.spotify.music",
			Restricted: true,
		},
	}
	whatsAppProc := ProcessInfo{
		PID:      5001,
		ExePath:  `C:\Program Files\WhatsApp\WhatsApp.exe`,
		BaseName: "WhatsApp.exe",
	}
	_, _, _, matchedWhatsApp := MatchProcess(whatsAppProc, reverseDnsTargets)
	if matchedWhatsApp {
		t.Errorf("D6.1 VIOLATION: Reverse-DNS target com.whatsapp falsely matched WhatsApp.exe on desktop!")
	}

	spotifyProc := ProcessInfo{
		PID:      5002,
		ExePath:  `C:\Users\test\AppData\Roaming\Spotify\Spotify.exe`,
		BaseName: "Spotify.exe",
	}
	_, _, _, matchedSpotify := MatchProcess(spotifyProc, reverseDnsTargets)
	if matchedSpotify {
		t.Errorf("D6.1 VIOLATION: Reverse-DNS target com.spotify.music falsely matched Spotify.exe on desktop!")
	}
}

func TestMatcher_CrossPlatformReverseDNSDoesNotMatchWindowsExe(t *testing.T) {
	// D6.1: Cross-platform reverse-DNS identifiers must NOT match desktop executables
	reverseDnsTargets := []TargetApp{
		{
			GoalID:     "g-android-whatsapp",
			AppID:      "com.whatsapp",
			Restricted: true,
		},
		{
			GoalID:     "g-android-spotify",
			AppID:      "com.spotify.music",
			Restricted: true,
		},
		{
			GoalID:     "g-android-discord",
			AppID:      "com.discord",
			Restricted: true,
		},
	}

	testCases := []struct {
		name    string
		proc    ProcessInfo
		comment string
	}{
		{
			name: "WhatsApp.exe vs com.whatsapp",
			proc: ProcessInfo{PID: 101, ExePath: `C:\Program Files\WhatsApp\WhatsApp.exe`, BaseName: "WhatsApp.exe"},
		},
		{
			name: "Spotify.exe vs com.spotify.music",
			proc: ProcessInfo{PID: 102, ExePath: `C:\Users\test\AppData\Roaming\Spotify\Spotify.exe`, BaseName: "Spotify.exe"},
		},
		{
			name: "Discord.exe vs com.discord",
			proc: ProcessInfo{PID: 103, ExePath: `C:\Users\test\AppData\Local\Discord\Discord.exe`, BaseName: "Discord.exe"},
		},
	}

	for _, tc := range testCases {
		_, _, _, matched := MatchProcess(tc.proc, reverseDnsTargets)
		if matched {
			t.Errorf("D6.1 VIOLATION: %s: reverse-DNS identifier falsely matched Windows executable %s", tc.name, tc.proc.BaseName)
		}
	}
}

