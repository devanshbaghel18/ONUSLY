# ONUSLY Android Enforcement App (D12 Release Candidate)

The ONUSLY Android app (`com.onusly.app`) provides native on-device blocking for restricted Android applications during active accountability goals, with strict typed target separation (`android_package` vs `desktop_app`).

---

## 1. Architecture & Enforcement Flow

```
                      +-----------------------------+
                      |   Backend Go REST API       |
                      |  GET /enforcement/blocklist |
                      +--------------+--------------+
                                     |
               HTTP GET (ETag / JWT) | 200 / 304
                                     v
                       +-------------+-------------+
                       |   EnforcementApiClient    |
                       +-------------+-------------+
                                     |
                                     v
                       +-------------+-------------+
                       |    BlocklistCache         |
                       |  (User-isolated local)    |
                       +-------------+-------------+
                                     |
                                     v
+------------------------+     +-----+-----+     +-----------------------+
|  User opens app        | --> |  Blocker  | --> | Launch LockActivity   |
| (TYPE_WINDOW_STATE_CHG)|     |  Engine   |     | (Full-screen blocking)|
+------------------------+     +-----------+     +-----------------------+
```

1. **Typed Target Separation Contract (D12 & D6.1):**
   - The Android client strictly consumes targets of kind `android_package` (e.g. `com.discord`, `com.reddit.frontpage`).
   - Targets of kind `desktop_app` (e.g. `discord.exe`, `slack`) are strictly ignored on Android.
   - Reverse-DNS desktop targets (e.g. `org.gnome.Calculator` on Linux) are never matched on Android.
   - Legacy policies lacking typed targets consume the `apps` field while filtering out desktop file extensions (`.exe`, `.desktop`, `.bat`, `.app`).

2. **Authentication & Session:**
   - Native Android Google Sign-In requests a Google ID token using the ONUSLY Web Client ID as `serverClientId`.
   - The token is exchanged via `POST /auth/google` on the backend, issuing an ONUSLY JWT.
   - The JWT is saved securely in local application preferences along with the authenticated user ID (`sub`).

3. **Blocklist Sync & Caching:**
   - Syncs via `GET /enforcement/blocklist` with `Authorization: Bearer <JWT>` and `If-None-Match: <version>`.
   - On `200 OK`: Validates that `response.userId` matches the authenticated user ID and saves the new blocklist with timestamp.
   - On `304 Not Modified`: Updates the last sync timestamp while preserving existing cached targets.
   - On network failure or server error: Maintains and continues enforcing the last successful cache (fail-safe / LKG).
   - On HTTP `403 Forbidden` (`DEVICE_REVOKED`): Clears local blocklist cache, resets state flows, halts active enforcement, and triggers user sign-out.
   - Multi-user isolation: User A's cache cannot be overwritten by User B without explicit clearing, and User B can never read User A's cache.

4. **Foreground Package Detection & Privacy Boundary:**
   - Implemented as an Android `AccessibilityService` (`OnuslyAccessibilityService`).
   - Configured with `canRetrieveWindowContent="false"` in `accessibility_service_config.xml`.
   - Does **NOT** inspect screen text, read window contents, or record keystrokes.
   - Detects only `AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED` and `TYPE_WINDOWS_CHANGED` package names.
   - Built-in loop protection: Never blocks ONUSLY itself, system launchers, emergency dialers, or system settings (`ALWAYS_ALLOWED_PACKAGES`).

5. **Lock Screen Enforcement:**
   - When a blocked package is detected, `LockActivity` is launched immediately as a full-screen blocking overlay.
   - Displays plain-text goal titles (sanitized against formatting injection).
   - Status indicators:
     - Active goals: *"Complete your goal and submit proof."*
     - Waiting for review (proof submitted): *"Proof submitted. Waiting for review."*
     - Rejected goals: *"Proof was rejected. Resubmit proof to lift restriction."*
   - Includes **"Check again"** button for immediate backend sync.
   - When the user presses Back, the app routes them to the Android Home screen (`Intent.CATEGORY_HOME`) to prevent popping back into the blocked app.

---

## 2. Requirements & Building

- **JDK:** Version 17 or Version 21 (Eclipse Temurin 21 recommended)
- **Android SDK:** Compile SDK 35, Min SDK 26, Target SDK 35, Build-Tools 36.0.0

### Build Release APK:
```bash
./gradlew assembleRelease
```
Output: `app/build/outputs/apk/release/app-release.apk`

### Build Debug APK:
```bash
./gradlew assembleDebug
```
Output: `app/build/outputs/apk/debug/app-debug.apk`

### Run Unit Tests:
```bash
./gradlew testDebugUnitTest
```
Runs 182 automated unit tests across 18 test suites covering:
- `D12TypedPolicyEnforcementTest`: Android package exact matching, desktop app target isolation, reverse-DNS desktop exclusion, legacy fallback, system app immunity, package format validation, ETag 304 durability, device revocation.
- `BlocklistParsingTest`: JSON parsing, whitespace normalization, and cache serialization.
- `BlocklistCacheTest`: User isolation, cross-user overwrite protection, cache expiration immunity, and clearing.
- `BlockerEngineTest`: Target status evaluation (`active`, `proof_submitted`, `rejected`, `completed`), package matching, and loop protection.
- `ApiClientTest`: MockWebServer ETag / 304 handling, error code propagation, and header verification.
- `GoalEditingValidationTest`, `GoalModelsParsingTest`, `ProofAndApprovalFlowTest`, `FriendsAndCommunitiesTest`, `RealtimeWebSocketLifecycleTest`, `Slice4SettingsAndDiagnosticsTest`, `Slice5SecurityHardeningTest`, `SystemAuditVerificationTest`, `TargetCatalogAndEnforcementTest`, `InstalledAppsManagerTest`, `CriticalWorkflowsTest`.

---

## 3. Important Android OS & OEM Considerations

### Android 13+ Restricted Settings
When installing the APK via `adb` or sideloading on Android 13 (API 33) or higher, Android blocks accessibility permissions by default:
1. Open Android **Settings** -> **Apps** -> **See all apps**.
2. Select **ONUSLY**.
3. Tap the **three-dot menu (⋮)** in the top right corner.
4. Select **"Allow restricted settings"**.
5. Authenticate with device PIN or fingerprint.
6. Return to **Accessibility Settings** -> **ONUSLY** and turn on the service.

### OEM Battery Optimization (Xiaomi, Samsung, OnePlus, Huawei)
To prevent the OS from killing the background accessibility service:
1. Long-press ONUSLY app icon -> **App info** -> **Battery**.
2. Set to **"Unrestricted"** (or disable "Battery Optimization").
3. On MIUI / ColorOS / OxygenOS: Enable **"Auto-start"** and lock the app in the recent apps tray.

---

## 4. Backend Configuration

- **Android Emulator:** Uses `http://10.0.2.2:8080` (routes directly to host `localhost:8080`).
- **Physical Device:** In the app, tap the Settings icon in the top right and update the Base URL to your host machine's LAN IP (e.g. `http://192.168.1.50:8080`).
