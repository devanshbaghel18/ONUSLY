import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  isProtectedProcess,
  normalizeDesktopAppTarget,
  inferTargetType,
  getFriendlyTargetName,
  getUniqueDisplayTargets,
} from "../src/lib/blockedApps.js";

describe("D6 Frontend Application & Target Control Plane Suite", () => {
  test("1. normalizeDesktopAppTarget trims whitespace, lowercases, and strips extensions", () => {
    assert.equal(normalizeDesktopAppTarget("  Discord.exe  "), "discord");
    assert.equal(normalizeDesktopAppTarget("Steam.EXE"), "steam");
    assert.equal(normalizeDesktopAppTarget("Spotify.desktop"), "spotify");
    assert.equal(normalizeDesktopAppTarget("blocked_app"), "blocked_app");
  });

  test("2. isProtectedProcess strictly identifies OS components and ONUSLY agent binaries", () => {
    // Windows core components
    assert.equal(isProtectedProcess("explorer.exe"), true);
    assert.equal(isProtectedProcess("Explorer"), true);
    assert.equal(isProtectedProcess("taskmgr.exe"), true);
    assert.equal(isProtectedProcess("cmd.exe"), true);
    assert.equal(isProtectedProcess("powershell.exe"), true);
    assert.equal(isProtectedProcess("pwsh.exe"), true);
    assert.equal(isProtectedProcess("csrss.exe"), true);
    assert.equal(isProtectedProcess("services.exe"), true);

    // Linux core components
    assert.equal(isProtectedProcess("systemd"), true);
    assert.equal(isProtectedProcess("init"), true);
    assert.equal(isProtectedProcess("bash"), true);
    assert.equal(isProtectedProcess("sh"), true);

    // ONUSLY binaries
    assert.equal(isProtectedProcess("onusly-agent.exe"), true);
    assert.equal(isProtectedProcess("onusly-native-host.exe"), true);
    assert.equal(isProtectedProcess("onusly-agent"), true);

    // Unprotected user applications
    assert.equal(isProtectedProcess("discord.exe"), false);
    assert.equal(isProtectedProcess("steam"), false);
    assert.equal(isProtectedProcess("blocked_app.exe"), false);
  });

  test("3. inferTargetType accurately differentiates desktop apps, domains, and Android packages", () => {
    assert.equal(inferTargetType("discord"), "desktop");
    assert.equal(inferTargetType("steam"), "desktop");
    assert.equal(inferTargetType("blocked_app"), "desktop");

    assert.equal(inferTargetType("youtube.com"), "domain");
    assert.equal(inferTargetType("news.ycombinator.com"), "domain");
    assert.equal(inferTargetType("https://twitter.com/home"), "domain");

    assert.equal(inferTargetType("com.snapchat.android"), "app");
    assert.equal(inferTargetType("com.google.android.youtube"), "app");
    assert.equal(inferTargetType("org.telegram.messenger"), "app");
  });

  test("4. getFriendlyTargetName resolves desktop apps and catalog aliases", () => {
    assert.equal(getFriendlyTargetName("discord"), "Discord");
    assert.equal(getFriendlyTargetName("com.discord"), "Discord");
    assert.equal(getFriendlyTargetName("steam"), "Steam");
    assert.equal(getFriendlyTargetName("spotify"), "Spotify");
    assert.equal(getFriendlyTargetName("youtube.com"), "YouTube");
    assert.equal(getFriendlyTargetName("blocked_app"), "blocked_app");
  });

  test("5. getUniqueDisplayTargets formats badges with explicit target types", () => {
    const targets = getUniqueDisplayTargets(
      ["discord", "com.snapchat.android"],
      ["steampowered.com", "news.ycombinator.com"]
    );

    assert.equal(targets.length, 4);

    const discord = targets.find((t) => t.label === "Discord");
    assert.ok(discord);
    assert.equal(discord.type, "desktop");

    const snapchat = targets.find((t) => t.label === "Snapchat");
    assert.ok(snapchat);
    assert.equal(snapchat.type, "app");

    const steam = targets.find((t) => t.label === "Steam");
    assert.ok(steam);
    assert.equal(steam.type, "domain");

    const customWeb = targets.find((t) => t.label === "news.ycombinator.com");
    assert.ok(customWeb);
    assert.equal(customWeb.type, "domain");
  });
});

