package apps

import (
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func TestAppPolicyManager_GoalStatusContract(t *testing.T) {
	mgr := NewAppPolicyManager()

	data := &core.BlocklistData{
		Version: "v1",
		Targets: []core.TargetItem{
			{
				GoalID:    "g-active",
				GoalTitle: "Block Discord",
				Status:    "active",
				Apps:      []string{"discord"},
			},
			{
				GoalID:    "g-proof",
				GoalTitle: "Block Steam",
				Status:    "proof_submitted",
				Apps:      []string{"steam"},
			},
			{
				GoalID:    "g-rejected",
				GoalTitle: "Block Spotify",
				Status:    "rejected",
				Apps:      []string{"spotify"},
			},
			{
				GoalID:    "g-completed",
				GoalTitle: "Allow Slack",
				Status:    "completed",
				Apps:      []string{"slack"},
			},
		},
	}

	mgr.UpdateFromBlocklist(data)

	// 1. active -> Restricted
	_, stDiscord, confDiscord, _ := mgr.EvaluateProcess(ProcessInfo{BaseName: "Discord.exe"})
	if stDiscord != StateRestricted || confDiscord == ConfidenceNone {
		t.Errorf("Expected active Discord to be StateRestricted, got %v (conf: %v)", stDiscord, confDiscord)
	}

	// 2. proof_submitted -> Restricted
	_, stSteam, confSteam, _ := mgr.EvaluateProcess(ProcessInfo{BaseName: "steam.exe"})
	if stSteam != StateRestricted || confSteam == ConfidenceNone {
		t.Errorf("Expected proof_submitted Steam to be StateRestricted, got %v (conf: %v)", stSteam, confSteam)
	}

	// 3. rejected -> Restricted
	_, stSpotify, confSpotify, _ := mgr.EvaluateProcess(ProcessInfo{BaseName: "Spotify.exe"})
	if stSpotify != StateRestricted || confSpotify == ConfidenceNone {
		t.Errorf("Expected rejected Spotify to be StateRestricted, got %v (conf: %v)", stSpotify, confSpotify)
	}

	// 4. completed -> Allowed
	_, stSlack, confSlack, _ := mgr.EvaluateProcess(ProcessInfo{BaseName: "slack.exe"})
	if stSlack != StateAllowed || confSlack == ConfidenceNone {
		t.Errorf("Expected completed Slack to be StateAllowed, got %v (conf: %v)", stSlack, confSlack)
	}

	// 5. Unrelated app -> Allowed
	_, stNotepad, confNotepad, _ := mgr.EvaluateProcess(ProcessInfo{BaseName: "notepad.exe"})
	if stNotepad != StateAllowed || confNotepad != ConfidenceNone {
		t.Errorf("Expected unlisted notepad to be StateAllowed with ConfidenceNone, got %v (conf: %v)", stNotepad, confNotepad)
	}

	// 6. Dynamic unblock: transition discord to "completed"
	data.Targets[0].Status = "completed"
	data.Version = "v2"
	mgr.UpdateFromBlocklist(data)

	_, stDiscordAfter, _, _ := mgr.EvaluateProcess(ProcessInfo{BaseName: "Discord.exe"})
	if stDiscordAfter != StateAllowed {
		t.Errorf("Expected Discord to transition to StateAllowed after completion, got %v", stDiscordAfter)
	}
}

func TestAppPolicyManager_TargetTypingAndCrossPlatformIsolation(t *testing.T) {
	mgr := NewAppPolicyManager()

	data := &core.BlocklistData{
		Version: "v1-typed",
		Targets: []core.TargetItem{
			{
				GoalID:    "g-android-whatsapp",
				GoalTitle: "Block Mobile WhatsApp Only",
				Status:    "active",
				Apps:      []string{"com.whatsapp"},
				ApplicationTargets: []core.ApplicationTarget{
					{Kind: "android_package", Identifier: "com.whatsapp"},
				},
			},
			{
				GoalID:    "g-desktop-discord",
				GoalTitle: "Block Desktop Discord",
				Status:    "active",
				Apps:      []string{"discord"},
				ApplicationTargets: []core.ApplicationTarget{
					{Kind: "desktop_app", Identifier: "discord"},
				},
			},
		},
	}

	mgr.UpdateFromBlocklist(data)

	// 1. WhatsApp.exe on desktop must NOT be restricted because com.whatsapp is an android_package
	_, stWhatsApp, confWhatsApp, reasonWhatsApp := mgr.EvaluateProcess(ProcessInfo{
		BaseName: "WhatsApp.exe",
		ExePath:  `C:\Program Files\WhatsApp\WhatsApp.exe`,
	})
	if stWhatsApp != StateAllowed {
		t.Fatalf("D6.1 VIOLATION: WhatsApp.exe restricted by android_package target! State: %v, Reason: %s", stWhatsApp, reasonWhatsApp)
	}
	if confWhatsApp != ConfidenceNone {
		t.Fatalf("Expected ConfidenceNone for unmonitored desktop app, got %v", confWhatsApp)
	}

	// 2. Discord.exe on desktop MUST be restricted because discord is a desktop_app
	_, stDiscord, confDiscord, _ := mgr.EvaluateProcess(ProcessInfo{
		BaseName: "Discord.exe",
		ExePath:  `C:\Users\test\AppData\Local\Discord\Discord.exe`,
	})
	if stDiscord != StateRestricted {
		t.Fatalf("Expected desktop_app target discord to restrict Discord.exe, got %v", stDiscord)
	}
	if confDiscord == ConfidenceNone {
		t.Fatalf("Expected valid confidence for desktop_app match, got %v", confDiscord)
	}
}

func TestD61_CompatibilityReleaseGate_DottedDesktopAndMobileSeparation(t *testing.T) {
	// D7 Phase A Release Gate:
	// Verify that:
	// 1. Explicit applicationTargets is authoritative
	// 2. desktop_app targets (including dotted IDs like org.gnome.Calculator and com.example.editor) are enforced on desktop
	// 3. android_package targets (com.whatsapp, com.spotify.music) are strictly ignored on desktop
	// 4. com.whatsapp never matches WhatsApp.exe
	// 5. com.spotify.music never matches Spotify.exe
	// 6. Desktop-only targets with empty Apps slices are not skipped
	mgr := NewAppPolicyManager()

	data := &core.BlocklistData{
		Version: "v1-d7-baseline",
		Targets: []core.TargetItem{
			{
				GoalID:    "g-mixed-targets",
				GoalTitle: "D6.1 Cross-Platform Compatibility Test Goal",
				Status:    "active",
				Apps: []string{
					// Only android packages in legacy apps field
					"com.whatsapp",
					"com.spotify.music",
				},
				ApplicationTargets: []core.ApplicationTarget{
					{Kind: "desktop_app", Identifier: "org.gnome.Calculator"},
					{Kind: "desktop_app", Identifier: "com.example.editor"},
					{Kind: "android_package", Identifier: "com.whatsapp"},
					{Kind: "android_package", Identifier: "com.spotify.music"},
					{Kind: "desktop_app", Identifier: "discord"},
					{Kind: "desktop_app", Identifier: "steam"},
				},
			},
			{
				GoalID:    "g-desktop-only-empty-apps",
				GoalTitle: "Desktop Only Goal with Empty Apps",
				Status:    "active",
				Apps:      []string{}, // Empty Apps slice
				ApplicationTargets: []core.ApplicationTarget{
					{Kind: "desktop_app", Identifier: "slack"},
				},
			},
		},
	}

	mgr.UpdateFromBlocklist(data)

	// Check 1: com.whatsapp must NEVER match WhatsApp.exe on desktop
	_, stWA, confWA, _ := mgr.EvaluateProcess(ProcessInfo{
		BaseName: "WhatsApp.exe",
		ExePath:  `C:\Program Files\WhatsApp\WhatsApp.exe`,
	})
	if stWA != StateAllowed || confWA != ConfidenceNone {
		t.Fatalf("CRITICAL REGRESSION: com.whatsapp matched WhatsApp.exe on desktop (state: %v, conf: %v)", stWA, confWA)
	}

	// Check 2: com.spotify.music must NEVER match Spotify.exe on desktop
	_, stSpotify, confSpotify, _ := mgr.EvaluateProcess(ProcessInfo{
		BaseName: "Spotify.exe",
		ExePath:  `C:\Users\test\AppData\Roaming\Spotify\Spotify.exe`,
	})
	if stSpotify != StateAllowed || confSpotify != ConfidenceNone {
		t.Fatalf("CRITICAL REGRESSION: com.spotify.music matched Spotify.exe on desktop (state: %v, conf: %v)", stSpotify, confSpotify)
	}

	// Check 3: Dotted desktop target org.gnome.Calculator must be restricted on desktop
	_, stCalc, confCalc, _ := mgr.EvaluateProcess(ProcessInfo{
		BaseName: "org.gnome.Calculator",
		ExePath:  "/usr/bin/org.gnome.Calculator",
	})
	if stCalc != StateRestricted || confCalc == ConfidenceNone {
		t.Fatalf("Expected dotted desktop_app target org.gnome.Calculator to be restricted, got state: %v, conf: %v", stCalc, confCalc)
	}

	// Check 4: Dotted desktop target com.example.editor must be restricted on desktop
	_, stEditor, confEditor, _ := mgr.EvaluateProcess(ProcessInfo{
		BaseName: "com.example.editor",
		ExePath:  "/usr/local/bin/com.example.editor",
	})
	if stEditor != StateRestricted || confEditor == ConfidenceNone {
		t.Fatalf("Expected dotted desktop_app target com.example.editor to be restricted, got state: %v, conf: %v", stEditor, confEditor)
	}

	// Check 5: discord must be restricted on desktop
	_, stDiscord, confDiscord, _ := mgr.EvaluateProcess(ProcessInfo{
		BaseName: "Discord.exe",
		ExePath:  `C:\Users\test\AppData\Local\Discord\app-1.0\Discord.exe`,
	})
	if stDiscord != StateRestricted || confDiscord == ConfidenceNone {
		t.Fatalf("Expected desktop_app target discord to be restricted, got state: %v, conf: %v", stDiscord, confDiscord)
	}

	// Check 6: steam must be restricted on desktop
	_, stSteam, confSteam, _ := mgr.EvaluateProcess(ProcessInfo{
		BaseName: "steam.exe",
		ExePath:  `C:\Program Files (x86)\Steam\steam.exe`,
	})
	if stSteam != StateRestricted || confSteam == ConfidenceNone {
		t.Fatalf("Expected desktop_app target steam to be restricted, got state: %v, conf: %v", stSteam, confSteam)
	}

	// Check 7: Desktop-only goal with empty Apps slice (slack) must be restricted
	_, stSlack, confSlack, _ := mgr.EvaluateProcess(ProcessInfo{
		BaseName: "slack.exe",
		ExePath:  `C:\Users\test\AppData\Local\slack\slack.exe`,
	})
	if stSlack != StateRestricted || confSlack == ConfidenceNone {
		t.Fatalf("Expected desktop-only goal with empty Apps slice (slack) to be restricted, got state: %v, conf: %v", stSlack, confSlack)
	}
}

