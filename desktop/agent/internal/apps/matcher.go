package apps

import (
	"path/filepath"
	"strings"
)

// Protected system processes that MUST NEVER be matched, restricted, or terminated.
var protectedProcessNames = map[string]bool{
	// Windows core system & shells
	"csrss.exe":                   true,
	"smss.exe":                   true,
	"wininit.exe":                true,
	"winlogon.exe":               true,
	"services.exe":               true,
	"lsass.exe":                  true,
	"svchost.exe":                true,
	"dwm.exe":                    true,
	"explorer.exe":               true,
	"taskmgr.exe":                true,
	"sihost.exe":                 true,
	"fontdrvhost.exe":            true,
	"searchhost.exe":             true,
	"shellexperiencehost.exe":    true,
	"startmenuexperiencehost.exe": true,
	"ctfmon.exe":                 true,
	"conhost.exe":                true,
	"cmd.exe":                    true,
	"powershell.exe":             true,
	"pwsh.exe":                   true,
	"wsmprovhost.exe":            true,
	"dllhost.exe":                true,

	// Linux core system & daemons
	"systemd":                    true,
	"init":                       true,
	"kthreadd":                   true,
	"dbus-daemon":                true,
	"systemd-journald":           true,
	"systemd-logind":             true,
	"systemd-resolved":           true,
	"systemd-udevd":              true,
	"xorg":                       true,
	"wayland":                    true,
	"gnome-shell":                true,
	"plasmashell":                true,
	"bash":                       true,
	"sh":                         true,
	"zsh":                        true,
	"login":                      true,
	"sshd":                       true,
	"cron":                       true,
	"polkitd":                    true,

	// ONUSLY Desktop Agent & Native Messaging Host
	"onusly-agent.exe":           true,
	"onusly-native-host.exe":     true,
	"onusly-agent":               true,
	"onusly-native-host":         true,
}

// Common consumer application catalog mapping canonical app IDs to typical executable names.
var knownAppExeCatalog = map[string][]string{
	"discord":       {"Discord.exe", "discord"},
	"steam":         {"steam.exe", "steamwebhelper.exe", "steam"},
	"spotify":       {"Spotify.exe", "spotify"},
	"telegram":      {"Telegram.exe", "telegram-desktop"},
	"slack":         {"slack.exe", "slack"},
	"epicgames":     {"EpicGamesLauncher.exe"},
	"battlenet":     {"Battle.net.exe"},
	"riotclient":    {"RiotClientServices.exe"},
	"leagueoflegends": {"LeagueClient.exe", "LeagueClientUx.exe"},
	"roblox":        {"RobloxPlayerBeta.exe"},
	"minecraft":     {"Minecraft.exe", "javaw.exe"},
	"whatsapp":      {"WhatsApp.exe"},
	"vlc":           {"vlc.exe", "vlc"},
	"obs":           {"obs64.exe", "obs32.exe", "obs"},
}

// NormalizeAppID sanitizes an application identifier or target string.
func NormalizeAppID(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimSuffix(s, ".exe")
	s = strings.TrimSuffix(s, ".desktop")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// IsProtectedSystemProcess returns true if the process is a critical system component or ONUSLY binary.
func IsProtectedSystemProcess(exePath, baseName string) bool {
	bn := strings.ToLower(baseName)
	if bn == "" && exePath != "" {
		bn = strings.ToLower(filepath.Base(exePath))
	}

	if protectedProcessNames[bn] {
		return true
	}

	// Protect Windows system directory binaries
	lowerPath := strings.ToLower(filepath.ToSlash(exePath))
	if strings.Contains(lowerPath, "/windows/system32/") || strings.Contains(lowerPath, "/windows/syswow64/") {
		// Only protect known core Windows executables, not third-party apps placed in system folders
		if protectedProcessNames[bn] {
			return true
		}
	}

	return false
}

// MatchProcess matches a runtime process against authoritative target applications with explicit precedence.
func MatchProcess(proc ProcessInfo, targets []TargetApp) (TargetApp, MatchConfidence, string, bool) {
	// Level 0: Immediate immunity check
	if IsProtectedSystemProcess(proc.ExePath, proc.BaseName) {
		return TargetApp{}, ConfidenceNone, "protected system process", false
	}

	procBase := strings.ToLower(proc.BaseName)
	if procBase == "" && proc.ExePath != "" {
		procBase = strings.ToLower(filepath.Base(proc.ExePath))
	}
	normProcBase := NormalizeAppID(procBase)

	for _, target := range targets {
		normTargetAppID := NormalizeAppID(target.AppID)

		// 1. Package Name Match (Highest confidence)
		if proc.Identity != nil && proc.Identity.PackageName != "" {
			if strings.EqualFold(proc.Identity.PackageName, target.AppID) {
				return target, ConfidencePackageID, "package name match", true
			}
			for _, alias := range target.Aliases {
				if strings.EqualFold(proc.Identity.PackageName, alias) {
					return target, ConfidencePackageID, "package alias match", true
				}
			}
		}

		// 2. AUMID Match (High confidence)
		if proc.Identity != nil && proc.Identity.AUMID != "" {
			if strings.EqualFold(proc.Identity.AUMID, target.AppID) {
				return target, ConfidenceAppID, "AUMID match", true
			}
		}

		// 3. Exact Executable Path Match (Strong confidence)
		if proc.ExePath != "" && proc.Identity != nil && proc.Identity.ExePath != "" {
			if strings.EqualFold(filepath.Clean(proc.ExePath), filepath.Clean(proc.Identity.ExePath)) {
				return target, ConfidenceExactPath, "exact executable path match", true
			}
		}

		// 4. Catalog Known Executables Match (Medium confidence)
		if knownExes, ok := knownAppExeCatalog[normTargetAppID]; ok {
			for _, exe := range knownExes {
				if strings.EqualFold(procBase, strings.ToLower(exe)) {
					return target, ConfidenceDirBinary, "known application catalog binary match", true
				}
			}
		}

		// 5. Direct BaseName match (Strict equality, no substrings or reverse-DNS prefix stripping)
		if normProcBase == normTargetAppID {
			return target, ConfidenceBaseName, "exact base name match", true
		}

		// 6. Configured Aliases
		for _, alias := range target.Aliases {
			normAlias := NormalizeAppID(alias)
			if normProcBase == normAlias {
				return target, ConfidenceBaseName, "target alias match", true
			}
		}
	}

	return TargetApp{}, ConfidenceNone, "", false
}

