# ONUSLY Android Enforcement App (Phase 3 - Slice A)

The ONUSLY Android app (`com.onusly.app`) provides native on-device blocking for restricted Android applications during active accountability goals.

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

1. **Authentication:**
   - Native Android Google Sign-In requests a Google ID token using the ONUSLY Web Client ID as `serverClientId`.
   - The token is exchanged via `POST /auth/google` on the backend, issuing an ONUSLY JWT.
   - The JWT is saved securely in local application preferences along with the authenticated user ID (`sub`).

2. **Blocklist Sync & Caching:**
   - Syncs via `GET /enforcement/blocklist` with `Authorization: Bearer <JWT>` and `If-None-Match: <version>`.
   - On `200 OK`: Validates that `response.userId` matches the authenticated user ID and saves the new blocklist with timestamp.
   - On `304 Not Modified`: Updates the last sync timestamp while preserving existing cached targets.
   - On network failure or server error: Maintains and continues enforcing the last successful cache (fail-safe).
   - Multi-user isolation: User A's cache cannot be overwritten by User B without explicit clearing, and User B can never read User A's cache.

3. **Foreground Package Detection:**
   - Implemented as an Android `AccessibilityService` (`OnuslyAccessibilityService`).
   - Configured with `canRetrieveWindowContent="false"`.
   - Does **NOT** inspect screen text, read window contents, or record keystrokes.
   - Detects only `AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED` package names.
   - Built-in loop protection: Never blocks ONUSLY itself, system launchers, or system settings.

4. **Lock Screen Enforcement:**
   - When a blocked package is detected, `LockActivity` is launched immediately as a full-screen blocking overlay.
   - Displays plain-text goal titles (sanitized against formatting injection).
   - Status indicators:
     - Active goals: *"Complete your goal and submit proof."*
     - Waiting for review (proof submitted): *"Proof submitted. Waiting for review."*
   - Includes **"Check again"** button for immediate backend sync.
   - When the user presses Back, the app routes them to the Android Home screen (`Intent.CATEGORY_HOME`) to prevent popping back into the blocked app.

---

## 2. Requirements & Building

- **JDK:** Version 17 or Version 21
- **Android SDK:** Compile SDK 35, Min SDK 26, Target SDK 35

### Build Debug APK:
```bash
./gradlew assembleDebug
```
Output: `app/build/outputs/apk/debug/app-debug.apk`

### Run Unit Tests:
```bash
./gradlew test
```
Runs 22 unit tests covering:
- `BlocklistParsingTest`: JSON parsing, whitespace normalization, and cache serialization.
- `BlocklistCacheTest`: User isolation, cross-user overwrite protection, cache expiration immunity, and clearing.
- `BlockerEngineTest`: Target status evaluation (`active`, `proof_submitted`, `completed`), package matching, and loop protection.
- `ApiClientTest`: MockWebServer ETag / 304 handling, error code propagation, and header verification.

---

## 3. Important Android OS & OEM Considerations

### Android 13+ Restricted Settings
When installing the debug APK via `adb` or sideloading on Android 13 (API 33) or higher, Android blocks accessibility permissions by default:
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
