# Design: App Tokens & Nextcloud Login Flow for Reva

**Status:** Draft  
**Author:** Jesse Geens  
**Branch:** `apptokens`  
**Date:** 2026-05-29

---

## 1. Background & Motivation

The Nextcloud desktop sync client authenticates to a server using a device-specific *app token* (also called *app password*): a long random credential scoped to one device that can be revoked independently of the user's primary password. The client acquires this token through the **Nextcloud Login Flow v2**, a browser-redirect-based protocol that avoids having the client ever see the user's real password.

Reva already has a partial implementation of app passwords (`pkg/appauth/`, `internal/grpc/services/applicationauth/`), but it is missing:

1. The **HTTP endpoints** that the sync client expects (Login Flow v2 initiation, poll, and OCS token management).
2. An **ephemeral session store** for the in-progress login flow state (poll tokens, RSA keypairs, etc.).
3. A **web login UI** that the browser uses during the flow.
4. The `/ocs/v2.php/core/apppassword` OCS endpoints used for token issuance and revocation.

This document describes the design for all of the above.

---

## 2. Goals

- Full Nextcloud Login Flow **v2** compatibility, so the official Nextcloud desktop/mobile clients can authenticate to Reva.
- Re-use the existing `appauth.Manager` interface and `ApplicationsAPI` gRPC service for persistent token storage.
- Be pluggable: the web login UI and the session storage backend must be swappable without changing the flow logic.
- Support graceful token revocation from both the client side (DELETE endpoint) and the server side (existing `InvalidateAppPassword`).

## 3. Non-Goals

- **Login Flow v1** (the `nc://` deep-link webview approach): v1 is deprecated and insecure (the client reads the password directly from the redirect URI). It will not be implemented, though the architecture leaves room for it.
- **Full OCS API compatibility**: only the token-related OCS endpoints are in scope; the broader OCS capabilities API is out of scope.
- **Web UI implementation**: this document specifies the integration contract only; the actual HTML/CSS is out of scope.

---

## 3. Nextcloud Login Flow v2 — Protocol Summary

### 3.1 Overview

```
Client                          Reva/Browser
  |                                  |
  | POST /index.php/login/v2         |
  |--------------------------------->|
  |   {poll.token, login URL}        |
  |<---------------------------------|
  |                                  |
  | (open login URL in browser)      |
  |      GET /login/v2/flow?token=.. |
  |--------------------------------->|
  |           (user authenticates)   |
  |      <app token generated>       |
  |<---------------------------------|
  |                                  |
  | (poll every ~1s)                 |
  | POST /login/v2/poll  token=..    |
  |--------------------------------->|
  |   HTTP 404 (not yet done)        |
  |<---------------------------------|
  |  ...                             |
  | POST /login/v2/poll  token=..    |
  |--------------------------------->|
  |   HTTP 200 {server,user,passwd}  |
  |<---------------------------------|
  |                                  |
  | (store app token, done)          |
```

### 3.2 Initiation — `POST /index.php/login/v2`

No authentication required. The server creates an ephemeral **login flow session** and returns:

```json
{
  "poll": {
    "token": "<pollToken>",
    "endpoint": "https://reva.example.com/index.php/login/v2/poll"
  },
  "login": "https://reva.example.com/index.php/login/v2/flow?token=<loginToken>"
}
```

- `pollToken`: 128-character cryptographically random string, given to the client, stored as `SHA-512(pollToken + serverSecret)`.
- `loginToken`: 128-character cryptographically random string, stored in plaintext, embedded in the `login` URL for the browser.
- Server also generates a **per-session RSA-2048 keypair**; the private key is encrypted using the `pollToken` as a passphrase before storage.

### 3.3 Browser Authentication — `GET /index.php/login/v2/flow`

The browser opens the `login` URL. The server looks up the session by `loginToken` and presents the user with an authentication UI (see Decision D5). After successful authentication the server:

1. Generates an app token via `GenerateAppPassword` on behalf of the authenticated user.
2. Encrypts the plaintext token with the session's RSA public key (OAEP/SHA-256 padding).
3. Stores the encrypted token and the user's login name in the session.
4. Shows a success page to the user.

### 3.4 Polling — `POST /index.php/login/v2/poll`

The client posts `token=<pollToken>` as `application/x-www-form-urlencoded`.

- While the session exists but the app token has not been deposited yet: `HTTP 404`.
- Once the app token is deposited:
  1. Retrieve the session by `SHA-512(pollToken + serverSecret)`.
  2. Decrypt the stored private key using `pollToken`.
  3. Decrypt the stored app token using the private key.
  4. Delete the session record.
  5. Return `HTTP 200`:

```json
{
  "server": "https://reva.example.com",
  "loginName": "alice",
  "appPassword": "yKTVA4zgxjfivy52WqD8kW3M2pKGQr6srmUXMipRdunxjPFripJn0GMfmtNOqOolYSuJ6sCN"
}
```

Each poll token is single-use: the record is deleted on first successful retrieval.

### 3.5 Daily Use — Basic Auth with App Token

The client authenticates all subsequent requests (WebDAV, OCS, etc.) using HTTP Basic Auth with `loginName:appPassword`. This flows through the existing `appauth` credential strategy and `appauth` auth manager already present in Reva.

### 3.6 Revocation

Client-side: `DELETE /ocs/v2.php/core/apppassword` with Basic Auth using the app token itself.  
Server-side: admin or user calls the existing `InvalidateAppPassword` gRPC method, or the management UI.

---

## 4. Reva Current State

### 4.1 What Exists

| Component | Location | Notes |
|---|---|---|
| `AppPassword` proto | `cs3/auth/applications/v1beta1` | Has: `password` (bcrypt hash), `token_scope`, `label`, `expiration`, `ctime`, `utime`, `user` |
| `appauth.Manager` interface | `pkg/appauth/appauth.go` | `GenerateAppPassword`, `ListAppPasswords`, `InvalidateAppPassword`, `GetAppPassword` |
| JSON backend | `pkg/appauth/manager/json/json.go` | File-backed, bcrypt hashing, in-memory cache |
| `applicationauth` gRPC service | `internal/grpc/services/applicationauth/` | Wraps Manager, exposes CS3 `ApplicationsAPI`; `GetAppPassword` is unprotected |
| `appauth` auth manager | `pkg/auth/manager/appauth/appauth.go` | Uses `GetUserByClaim` + `GetAppPassword` via gateway |
| HTTP auth interceptor | `internal/http/interceptors/auth/auth.go` | Pluggable credential chain; `appauth` type supported via `basic` strategy |

### 4.2 What Is Missing

1. HTTP handlers for Login Flow v2 initiation, browser redirect, and polling.
2. Ephemeral session store for in-progress flows.
3. A web login page (or OIDC redirect) used during the flow.
4. OCS-compatible endpoints: `GET /ocs/v2.php/core/getapppassword` and `DELETE /ocs/v2.php/core/apppassword`.
5. The `appauth` credential strategy is currently handled as `basic` auth; we may need the client to be able to signal it is using an app token (for routing to the right auth manager).

### 4.3 Key Observation: Existing Auth Works

Once a client has an app token, the existing HTTP auth interceptor + `appauth` auth manager + `appauth` credential strategy already handles authentication correctly — `username:apptoken` over Basic Auth goes through `GetUserByClaim` → `GetAppPassword` → bcrypt verify → return user + scopes. **No changes to the authentication path are needed for the ongoing sync.** The new work is entirely in the token _issuance_ path.

---

## 5. Design Decisions

### D1: Login Flow Version Support

**Options:**

| Option | Description | Pros | Cons |
|---|---|---|---|
| **v2 only** | Implement only the `POST /index.php/login/v2` flow | Modern, secure, browser-agnostic; no client passwords in Reva | Older v1-only clients cannot connect |
| v1 only | Implement only the `nc://` deep-link webview flow | Simpler (no polling, no RSA) | Deprecated; client reads plaintext password; no app password isolation |
| Both | Implement v1 + v2 | Maximum compatibility | Extra maintenance burden; v1 has poor security properties |

**Decision: v2 only.**

Rationale: The Nextcloud desktop client has supported v2 since version 3.x (2020). Login Flow v1 is explicitly deprecated in Nextcloud documentation and requires the server to expose the user's actual password (or a credential equivalent) in a redirect URI. Implementing v1 would contradict the core goal of app token isolation.

If a future need arises for v1, the architecture can be extended — the session store and HTTP handler structure are independent.

---

### D2: Ephemeral Session Storage

The Login Flow v2 requires short-lived (≤20 min) records that hold the poll token hash, login token, RSA keypair, and (once the user authenticates) the encrypted app password.

**Options:**

| Option | Description | Pros | Cons |
|---|---|---|---|
| **A: In-memory map** | Goroutine-safe `sync.Map` or mutex-guarded map | Zero deps; fast; simple | Lost on restart; not multi-instance |
| B: File-backed (JSON) | Write sessions to a JSON file on each state change | Survives restarts | Slow; file locking; not multi-instance |
| C: SQL database | New table in a DB | Multi-instance; durable; queryable | New dependency; ops overhead |
| D: Pluggable interface | Define `LoginFlowSessionStore` interface; ship A as default | Extensible; default works for single-instance | More upfront code |

**Decision: Option D — define a `LoginFlowSessionStore` interface with an in-memory default.**

Rationale: Most Reva deployments are single-instance today. An in-memory default is sufficient and introduces zero new dependencies. The interface boundary (see §6.3) lets a future SQL or Redis backend be dropped in. Sessions that survive a restart (20-minute window) are not critical — the client just re-initiates the flow.

The interface has a hard TTL: any session older than a configurable `session_ttl` (default 20 minutes) is garbage-collected.

---

### D3: App Token Storage

Once the login flow completes, the app token needs to be stored durably so `GetAppPassword` can validate it on every request.

**Options:**

| Option | Description | Pros | Cons |
|---|---|---|---|
| **A: Existing JSON appauth manager** | Reuse `pkg/appauth/manager/json/json.go` as-is | Zero new code; already pluggable | File I/O; mutex contention at scale; no DB transactions |
| B: New SQL backend | Add `pkg/appauth/manager/sql/` | Multi-instance; indexable; atomic operations | New code + migration; new dep |
| C: Delegate to upstream IdP | Never store tokens in Reva; pass through to e.g. Nextcloud | Works if Nextcloud is the user store | Requires Nextcloud; defeats purpose of Reva-native flow |

**Decision: Option A, with a note that a SQL backend should be added as a follow-up.**

Rationale: The existing JSON manager works correctly and is already wired into the gateway via `ApplicationsAPI`. Adding a SQL backend is a separate task with its own schema decisions. The design document for this feature should not block on it; however the interface is already set up to accept it.

**Concern with JSON manager:** The bcrypt comparison in `GetAppPassword` iterates over all of a user's app passwords doing a bcrypt compare for each. At bcrypt cost 11 this is ~400ms per compare. For a user with many tokens this becomes a bottleneck. Mitigation: cap the number of app tokens per user (configurable, default 30), and document the issue for a SQL-backed implementation.

---

### D4: Credential Encryption in the Login Flow

Nextcloud uses an RSA-2048 keypair per session. The private key is encrypted with the poll token before storage. The plaintext app password is encrypted with the public key before storage. The client decrypts by: (1) decrypting the private key with the poll token, (2) decrypting the app password with the private key.

**Why bother with RSA if we have TLS?**

Nextcloud's rationale: the poll endpoint is unauthenticated. If a database/file is compromised, the attacker cannot extract app passwords without also knowing the poll tokens (which are never persisted). The poll token is in the client's memory only during the flow.

**Options:**

| Option | Description | Pros | Cons |
|---|---|---|---|
| **A: RSA-2048 per session (Nextcloud-compatible)** | Generate RSA keypair; encrypt private key with poll token; encrypt app token with public key | Defense in depth; storage compromise doesn't reveal tokens | Adds `crypto/rsa` usage; key generation latency (~5ms) |
| B: AES-GCM only | Encrypt the app token with `AES-GCM(key=SHA256(pollToken))` | Simpler; faster | Less alignment with Nextcloud reference; no asymmetric protection |
| C: No encryption (rely on TLS) | Store app token in plaintext in the session record | Simplest | Storage compromise reveals all pending tokens |

**Decision: Option A — RSA-2048 per session, matching Nextcloud's approach.**

Rationale: The security model is sound and the overhead is acceptable. Alignment with the Nextcloud reference implementation eases future debugging and compatibility testing. We use OAEP with SHA-256 (more modern than Nextcloud's PKCS1v15).

```go
// Key generation at session creation
privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
pubKeyBytes, _ := x509.MarshalPKIXPublicKey(&privKey.PublicKey)

// Encrypt private key with pollToken for storage
privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privKey)})
encryptedPrivKey := aesGCMEncrypt(privKeyPEM, []byte(pollToken))

// When app token arrives, encrypt with public key
encryptedAppToken, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, &privKey.PublicKey, []byte(appToken), nil)

// Poll: decrypt private key, then app token
privKeyPEM = aesGCMDecrypt(encryptedPrivKey, []byte(pollToken))
privKey = parsePEM(privKeyPEM)
appToken = rsa.DecryptOAEP(sha256.New(), rand.Reader, privKey, encryptedAppToken, nil)
```

> Note: Nextcloud uses `openssl_public_encrypt` with `OPENSSL_PKCS1_OAEP_PADDING`. We use Go's `rsa.EncryptOAEP` which is equivalent.

---

### D5: Web Login UI

The `GET /index.php/login/v2/flow?token=<loginToken>` endpoint must present an authentication UI to the user.

**Options:**

| Option | Description | Pros | Cons |
|---|---|---|---|
| **A: Redirect to OIDC provider** | Treat the flow as an OIDC authorization request; redirect to the configured IdP | No HTML in Reva; works with any OIDC IdP; single sign-on experience | Requires OIDC configuration; doesn't work for basic-auth-only deployments |
| B: Embedded HTML form | Reva serves a simple username/password HTML form | Works with any auth backend; no external deps | HTML/CSS to maintain; security scrutiny; no SSO |
| C: Configurable per deployment | Interface + config; ship both A and B; operator chooses | Maximum flexibility | More code; more configuration |
| D: Redirect to external URL | Operator configures a URL; Reva redirects there; the external app handles auth and calls back | Total separation of concerns | Requires a companion web app; complex callback handling |

**Decision: Option C — pluggable, ship OIDC redirect (A) and embedded form (B).**

Rationale: Reva deployments are heterogeneous. CERN deployments use OIDC/Keycloak; smaller deployments may use LDAP with basic auth. Both use cases must be supported without forking.

The two built-in handlers share a common interface:

```go
// WebLoginHandler handles the browser-facing part of the login flow.
type WebLoginHandler interface {
    // ServeLoginPage renders the login UI or redirects for a given loginToken.
    ServeLoginPage(w http.ResponseWriter, r *http.Request, loginToken string)
}
```

The OIDC handler:
- Initiates an OIDC Authorization Code flow with the `loginToken` encoded in the `state` parameter.
- On the OIDC callback, validates the ID token, resolves the Reva user, generates an app token, and deposits it in the session.

The form handler:
- Renders a minimal HTML form.
- On POST, authenticates with the configured credential strategy (basic/LDAP/etc.), generates an app token, deposits it.
- Should use a CSRF token (stored in a short-lived cookie tied to the `loginToken`).

---

### D6: App Token Scope

What scopes should newly-issued app tokens carry?

**Options:**

| Option | Description | Pros | Cons |
|---|---|---|---|
| **A: Full `user` scope** | Token has the same permissions as the user | Matches Nextcloud behavior; sync clients need full access | Tokens can do anything the user can |
| B: Configurable scope at issuance | The Login Flow allows the client to request specific scopes | Principle of least privilege | Nextcloud clients don't send scope requests; needs protocol extension |
| C: Read-only by default, upgradeable | Token can only read; upgrade requires additional user consent | Better security | Breaks sync clients that write |

**Decision: Option A — full `user` scope.**

Rationale: The Nextcloud sync client requires full read/write access (upload, move, delete, share). The scope system in Reva's `user.go` scope returns `true` for all resource accesses, making this consistent with how interactive user sessions work. A future Login Flow extension could allow scope negotiation (Decision D6 can be revisited without architectural changes because `GenerateAppPassword` already accepts a `scope` map).

---

### D7: HTTP Service Architecture

Where do the new endpoints live?

**Options:**

| Option | Description | Pros | Cons |
|---|---|---|---|
| **A: New `loginflowv2` HTTP service** | `internal/http/services/loginflowv2/` | Clean separation; independently configurable | New service to register and document |
| B: Add to existing OCS handler | Append routes to `internal/http/services/ocsapi/` | Fewer services | OCS handler is already large; Login Flow is not OCS |
| C: Add to existing WebDAV service | Add as a sub-handler | — | Conceptually wrong; WebDAV is file operations |

**Decision: Option A — new `loginflowv2` HTTP service, plus a thin `ocsapppassword` handler for the OCS token endpoints.**

The Login Flow v2 endpoints (`/index.php/login/v2`, `/index.php/login/v2/poll`, `/index.php/login/v2/flow`) are conceptually distinct from OCS, so they get their own service. The OCS token endpoints (`/ocs/v2.php/core/getapppassword`, `/ocs/v2.php/core/apppassword`) are small enough to be added as a sub-handler in a new `ocsapppassword` service or appended to the existing OCS service depending on how the OCS service is structured.

---

### D8: Token Labeling and Device Identity

How is the device name captured and stored in the app token label?

**Options:**

| Option | Description |
|---|---|
| **A: Use `User-Agent` from initiation request** | The client's User-Agent header at `POST /index.php/login/v2` becomes the label |
| B: Prompt user during web login | The web login page asks the user to name the device |
| C: Client sends a `deviceName` parameter | Non-standard extension to the Login Flow v2 protocol |

**Decision: Option A, with Option B as a UI enhancement.**

The `User-Agent` is always available and provides reasonable device identification (e.g., `Nextcloud-desktop/3.14 (Linux)`). The web login page can optionally display a text field pre-populated with the parsed User-Agent, allowing the user to change it. The final label used is whatever the web handler stores in the session; the flow handler itself just stores what the web handler sets.

---

## 6. Architecture

### 6.1 Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         Reva HTTP Server                        │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │               loginflowv2 service                        │   │
│  │  POST /index.php/login/v2          → InitiateHandler     │   │
│  │  GET  /index.php/login/v2/flow     → WebLoginHandler     │   │
│  │  POST /index.php/login/v2/poll     → PollHandler         │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │            ocsapppassword service                        │   │
│  │  GET  /ocs/v2.php/core/getapppassword → IssueHandler     │   │
│  │  DELETE /ocs/v2.php/core/apppassword  → RevokeHandler    │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                  │
│  ┌───────────────────────────▼──────────────────────────────┐   │
│  │          LoginFlowSessionStore interface                  │   │
│  │          (default: in-memory)                            │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                  HTTP Auth Interceptor                    │  │
│  │  (unchanged — appauth basic credential strategy works)   │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                    gRPC (gateway)
                              │
              ┌───────────────▼───────────────┐
              │     ApplicationsAPI           │
              │  GenerateAppPassword          │
              │  GetAppPassword               │
              │  InvalidateAppPassword        │
              │  ListAppPasswords             │
              └───────────────────────────────┘
                              │
              ┌───────────────▼───────────────┐
              │       appauth.Manager          │
              │  (JSON / SQL / pluggable)      │
              └───────────────────────────────┘
```

### 6.2 Sequence Diagram — Full Login Flow v2

```
Client          Browser         loginflowv2 svc    Gateway / AppAuth
  |                |                   |                   |
  |POST /login/v2  |                   |                   |
  |--------------->|                   |                   |
  |                | (redirect)        |                   |
  |                |GET /login/v2/flow?|                   |
  |                |token=<loginToken> |                   |
  |                |------------------>|                   |
  |                | [web login UI]    |                   |
  |                |<------------------|                   |
  |                | (user submits)    |                   |
  |                |POST credentials   |                   |
  |                |------------------>|                   |
  |                |                   |Authenticate(user) |
  |                |                   |------------------>|
  |                |                   |<------------------|
  |                |                   |GenerateAppPassword|
  |                |                   |------------------>|
  |                |                   |   appToken        |
  |                |                   |<------------------|
  |                |                   | encrypt(appToken) |
  |                |                   | store in session  |
  |                | [success page]    |                   |
  |                |<------------------|                   |
  |                                    |                   |
  |POST /login/v2/poll token=<poll>    |                   |
  |------------------------------------>                   |
  |  HTTP 200 {server,loginName,pass}  |                   |
  |<------------------------------------|                   |
  |                                                        |
  | (ongoing sync — Basic Auth username:appToken)          |
  |POST /remote.php/dav/files/alice/   HTTP auth intercept |
  |------------------------------------------------------->|
```

### 6.3 Data Models

#### LoginFlowSession (ephemeral, not persisted by default)

```go
type LoginFlowSession struct {
    // PollTokenHash is SHA-512(pollToken + serverSecret), used as the map key.
    PollTokenHash string

    // LoginToken is the plaintext token embedded in the browser URL.
    LoginToken string

    // ClientName is the User-Agent from the initiation request.
    ClientName string

    // CreatedAt is used for TTL enforcement.
    CreatedAt time.Time

    // PublicKeyPEM is the DER-encoded RSA public key.
    PublicKeyPEM []byte

    // EncryptedPrivateKey is the RSA private key encrypted with AES-GCM
    // using the poll token as the key material.
    EncryptedPrivateKey []byte

    // --- populated after user authenticates ---

    // LoginName is the authenticated user's login name.
    LoginName string

    // EncryptedAppToken is rsa.EncryptOAEP(publicKey, appToken).
    EncryptedAppToken []byte
}
```

#### LoginFlowSessionStore interface

```go
type LoginFlowSessionStore interface {
    // Create stores a new session and returns the poll token and login token.
    Create(clientName string) (pollToken, loginToken string, err error)

    // GetByLoginToken retrieves a session for the browser login step.
    GetByLoginToken(loginToken string) (*LoginFlowSession, error)

    // DepositCredentials stores the encrypted app token after user auth completes.
    DepositCredentials(loginToken, loginName string, encryptedAppToken []byte) error

    // Poll attempts to retrieve and delete a completed session.
    // Returns (nil, nil) if the session exists but credentials are not yet deposited.
    // Returns (nil, ErrNotFound) if the session does not exist (expired or already consumed).
    Poll(pollToken string) (*LoginFlowCredentials, error)

    // Cleanup removes expired sessions. Called periodically.
    Cleanup() int
}

type LoginFlowCredentials struct {
    Server      string
    LoginName   string
    AppPassword string // plaintext, only ever in memory
}
```

### 6.4 New Packages and Files

```
internal/http/services/loginflowv2/
    loginflowv2.go          # service registration, config, route setup
    initiate.go             # POST /index.php/login/v2
    poll.go                 # POST /index.php/login/v2/poll
    flow.go                 # GET  /index.php/login/v2/flow (delegates to WebLoginHandler)

internal/http/services/loginflowv2/weblogin/
    weblogin.go             # WebLoginHandler interface
    oidc/oidc.go            # OIDC redirect handler
    form/form.go            # embedded HTML form handler
    form/login.html         # minimal login page template

internal/http/services/ocsapppassword/
    ocsapppassword.go       # GET/DELETE /ocs/v2.php/core/apppassword(?)

pkg/loginflow/
    session.go              # LoginFlowSession struct
    store.go                # LoginFlowSessionStore interface
    memory/memory.go        # in-memory default implementation
```

---

## 7. HTTP API Specification

### 7.1 Initiate Login Flow

```
POST /index.php/login/v2
Content-Type: (any)
User-Agent: Nextcloud-desktop/3.14 (Linux)

(no body required)
```

Response `200 OK`:
```json
{
  "poll": {
    "token": "<128-char pollToken>",
    "endpoint": "https://reva.example.com/index.php/login/v2/poll"
  },
  "login": "https://reva.example.com/index.php/login/v2/flow?token=<128-char loginToken>"
}
```

Error cases: none (always returns 200 or 500 on internal error).

This endpoint must be in the HTTP interceptor's **unprotected endpoints** list.

### 7.2 Browser Login Page

```
GET /index.php/login/v2/flow?token=<loginToken>
```

Behavior depends on the configured `web_login_handler`:
- `oidc`: 302 redirect to the IdP's authorization endpoint with `state=<loginToken>`.
- `form`: 200 with HTML login form.

On authentication success: stores credentials in the session, returns a 200 success page.  
On authentication failure: returns 401 with the login form re-rendered with an error.

This endpoint must be in the **unprotected endpoints** list.

### 7.3 Poll

```
POST /index.php/login/v2/poll
Content-Type: application/x-www-form-urlencoded

token=<pollToken>
```

Response while waiting: `404 Not Found` (empty body).

Response on success: `200 OK`
```json
{
  "server": "https://reva.example.com",
  "loginName": "alice",
  "appPassword": "<plaintext app token>"
}
```

Response if session expired or already consumed: `404 Not Found`.

This endpoint must be in the **unprotected endpoints** list.

### 7.4 OCS: Exchange Password for App Password

```
GET /ocs/v2.php/core/getapppassword
Authorization: Basic alice:<real-or-existing-app-password>
OCS-APIRequest: true
```

Response `200 OK` (OCS XML format):
```xml
<?xml version="1.0"?>
<ocs>
  <meta>
    <status>ok</status>
    <statuscode>200</statuscode>
  </meta>
  <data>
    <apppassword>yKTVA4zgxjfivy52WqD8kW3M2pKGQr6s</apppassword>
  </data>
</ocs>
```

Or JSON if `Accept: application/json`:
```json
{"ocs":{"meta":{"status":"ok","statuscode":200},"data":{"apppassword":"yKTVA4zgx..."}}}
```

This endpoint **requires** authentication (uses the existing auth interceptor). The `User-Agent` is used as the label.

### 7.5 OCS: Delete App Password

```
DELETE /ocs/v2.php/core/apppassword
Authorization: Basic alice:<app-password-to-revoke>
OCS-APIRequest: true
```

Response `200 OK` (same OCS envelope, empty data).

This endpoint **requires** authentication. The password being used for authentication is the one that will be deleted — the handler must extract it from the Basic Auth credentials, not from a request body.

---

## 8. Configuration

```toml
[http.services.loginflowv2]
# Gateway service address
gatewaysvc = "localhost:9142"

# Server base URL returned in poll responses and the login URL
server_url = "https://reva.example.com"

# TTL for in-progress login flow sessions
session_ttl = "20m"

# Session store: "memory" (default) or future: "sql", "redis"
session_store = "memory"

# Shared secret mixed into poll token hashing (HMAC key material)
# Should be a long random string; can be shared across instances if clustered.
server_secret = "change-me-in-production"

# Which web login handler to use: "oidc" or "form"
web_login_handler = "oidc"

  [http.services.loginflowv2.web_login_handler_config]
  # For "oidc":
  issuer   = "https://keycloak.example.com/realms/myrealm"
  clientid = "reva-loginflow"
  # clientsecret is optional for public clients

  # For "form":
  # auth_type = "basic"  # credential strategy to use when validating form submissions
```

```toml
[http.services.ocsapppassword]
gatewaysvc = "localhost:9142"
# Max app passwords per user (prevents unbounded bcrypt iteration)
max_tokens_per_user = 30
```

---

## 9. Security Considerations

### 9.1 Poll Token Entropy

Both the poll token and the login token are 128-character random alphanumeric strings generated via `crypto/rand`. Entropy ≈ 128 × log₂(62) ≈ 761 bits. This is far beyond any brute-force attack.

### 9.2 Storage Compromise

Session records store:
- The *hash* of the poll token (not the plaintext) — an attacker with DB access cannot use the hash to poll.
- The RSA private key *encrypted* with the poll token — the private key is only useful if the attacker also knows the poll token.
- The app token *encrypted* with the RSA public key — only decryptable with the (encrypted) private key.

This three-layer protection means a storage compromise alone does not reveal any credentials.

### 9.3 CSRF in the Web Login Form

The embedded form handler must generate a CSRF token:
- On `GET /login/v2/flow`, generate `csrfToken = random(32 bytes)` and store it as a short-lived cookie (`SameSite=Strict`, `HttpOnly`, `Secure`, TTL=10m).
- On `POST` (form submission), verify the submitted `csrfToken` field matches the cookie.

The OIDC handler is not vulnerable to CSRF (the `state` parameter carries the `loginToken` and the redirect URL is validated by the IdP).

### 9.4 Token Expiry

App tokens do not expire by default (matching Nextcloud behavior — users manage them through the web UI). However, `GenerateAppPassword` accepts an `expiration` field, so an operator can configure a maximum lifetime via a policy.

Login flow sessions expire after `session_ttl` (default 20 minutes). Expired sessions are cleaned up by a background goroutine.

### 9.5 Rate Limiting

The poll endpoint (`POST /login/v2/poll`) and the initiation endpoint (`POST /index.php/login/v2`) are unauthenticated and could be abused. Rate limiting is out of scope for this feature but should be addressed at the infrastructure layer (e.g., nginx `limit_req`).

### 9.6 Logout and Token Revocation

The Nextcloud client calls `DELETE /ocs/v2.php/core/apppassword` on sign-out. The handler must:
1. Extract the Basic Auth credentials from the request.
2. Verify they authenticate successfully (avoid deleting someone else's token).
3. Call `InvalidateAppPassword` on the gateway.

Because the password is already validated by the HTTP auth interceptor, step 2 is implicit — an unauthenticated request would be rejected before reaching the handler.

### 9.7 Transport Security

All endpoints should be served over HTTPS in production. The embedded HTML form must set `<form action="...">` to an HTTPS URL. The OIDC redirect must use HTTPS. HTTP-only deployments are an operator responsibility (Reva does not enforce HTTPS at the application layer).

---

## 10. OIDC Web Login Handler — Detailed Flow

```
Browser                    loginflowv2 (Reva)            OIDC IdP
  |                               |                          |
  | GET /login/v2/flow?token=T    |                          |
  |------------------------------>|                          |
  |  302 to IdP auth URL          |                          |
  |  state = HMAC(T, stateSecret) |                          |
  |<------------------------------|                          |
  | GET /authorize?...&state=S    |                          |
  |------------------------------------------------------>|  |
  |              (user logs in)                           |  |
  | GET /login/v2/oidc/callback?code=C&state=S            |  |
  |<------------------------------------------------------|  |
  |------------------------------>|                          |
  |                               | exchange code for tokens |
  |                               |------------------------->|
  |                               | id_token, access_token   |
  |                               |<-------------------------|
  |                               | verify id_token          |
  |                               | GetUserByClaim(sub/email)|
  |                               | GenerateAppPassword      |
  |                               | DepositCredentials       |
  | 200 "Login successful"        |                          |
  |<------------------------------|                          |
```

The `state` parameter encodes the `loginToken` so the callback can look up the right session. It is HMAC-protected to prevent session fixation:

```go
state = base64(loginToken + "." + HMAC-SHA256(loginToken, stateSecret))
```

The OIDC callback endpoint (`GET /index.php/login/v2/oidc/callback`) must also be in the unprotected list.

---

## 11. Form Web Login Handler — Detailed Flow

```
Browser                    loginflowv2 (Reva)
  |                               |
  | GET /login/v2/flow?token=T    |
  |------------------------------>|
  | 200 <html> login form         |
  |   hidden field: loginToken=T  |
  |   hidden field: csrfToken=C   |
  |   Set-Cookie: csrf=C; ...     |
  |<------------------------------|
  | (user enters username/pass)   |
  | POST /login/v2/flow           |
  |   loginToken=T                |
  |   csrfToken=C                 |
  |   username=alice              |
  |   password=secret             |
  |------------------------------>|
  | verify csrfToken              |
  | Authenticate(alice, secret)   |
  | GenerateAppPassword           |
  | DepositCredentials(T, ...)    |
  | 200 "Login successful"        |
  |<------------------------------|
```

The form POST endpoint must be unprotected from the HTTP auth interceptor (it handles its own auth). It should respond with a success page rather than a redirect to avoid exposing the app token in browser history.

---

## 12. Implementation Order

1. **`pkg/loginflow/`** — session struct, store interface, in-memory implementation, cleanup goroutine.
2. **`internal/http/services/loginflowv2/`** — service skeleton, initiation handler, poll handler; wire to gateway for `GenerateAppPassword`.
3. **Form-based web login handler** — minimal HTML, CSRF, Basic-Auth form submission.
4. **`internal/http/services/ocsapppassword/`** — `getapppassword` and `DELETE apppassword` endpoints.
5. **OIDC web login handler** — OIDC redirect + callback; `stateSecret` HMAC validation.
6. **Integration tests** — full Login Flow v2 round-trip with the memory store; OCS endpoint tests.
7. **Documentation** — configuration reference, deployment guide.

---

## 13. Token Lifecycle — Format, Storage, Hashing, and Revocation

This section answers the core questions: what does a token look like, how is it stored, how is it validated efficiently, and how is it revoked.

### 13.1 Token Format: Random String vs JWT

**Why not JWTs for app tokens:**

JWTs are the right tool for Reva's short-lived _session_ tokens (the internal `X-Access-Token` the JWT manager mints after each authentication). They are the wrong tool for long-lived, device-bound, revocable credentials. Specifically:

| Property | JWT | Opaque random token |
|---|---|---|
| Revocability | ❌ Requires a server-side denylist | ✅ Delete the record |
| Server-side state | Required anyway (for denylist) | Required (for validation) |
| Payload visibility | Payload readable by anyone (base64) | Completely opaque |
| Validation speed | Fast (signature check only) | Fast (single hash lookup) |
| Token length | ~200–500 chars | ~48 chars |
| Expiry enforcement | In-band (JWT `exp` claim) | Out-of-band (compare to stored `expiration`) |

The decisive issue: if you have a JWT denylist you need server-side state anyway, so you gain nothing over an opaque token. Opaque tokens are also shorter, carry no information, and have no information-leakage risk if logged accidentally.

**Verdict: opaque random string.**

---

### 13.2 Token Generation

**Format:**
```
rvat_<base64url(32 random bytes)>
```

Example:
```
rvat_X7kLmN2pQr8sT1uV4wYzA5bC6dE9fG0h
```

Breaking this down:
- `rvat_` — "Reva App Token" prefix. This is not cosmetic: secret-scanning tools (truffleHog, GitHub secret scanning, Vault Sentinel) use known prefixes to detect leaked credentials in logs, source code, and CI output. The prefix tells them "this is a Reva app token and should be treated as a secret."
- 32 random bytes from `crypto/rand` = 256 bits of entropy. This makes brute-force and guessing attacks computationally infeasible even with a fast hash.
- base64url (no padding) = 43 characters. Total token length: 48 characters.

**Go generation snippet:**
```go
func generateToken() (string, error) {
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return "rvat_" + base64.RawURLEncoding.EncodeToString(b), nil
}
```

**Current implementation vs proposed:**

The current JSON backend uses `github.com/sethvargo/go-password` to generate a 16-character alphanumeric string (62^16 ≈ 2^95 bits entropy). This is adequate but:
- Has no type-identifying prefix (cannot be scanned for leaks).
- 95 bits is tighter than NIST recommends for long-lived credentials (≥128 bits).
- The `token_strength` config is poorly named — it controls character count, not bit strength, leading to confusion.

---

### 13.3 Hashing Scheme: bcrypt vs HMAC-SHA256

**The bcrypt problem:**

The current JSON backend stores and looks up app tokens using bcrypt:

```go
// validation (pkg/appauth/manager/json/json.go)
for hash, pw := range appPassword {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    if err == nil { ... }
}
```

This is O(n × bcrypt_cost) per authentication request, where n is the number of tokens the user has. At bcrypt cost 11, each comparison takes ~300ms. With 10 tokens, auth takes up to 3 seconds.

**Why bcrypt was chosen (and why it's wrong here):**

Bcrypt is designed for _low-entropy human passwords_. Its intentional slowness raises the cost of offline dictionary attacks. For a password like `hunter2`, bcrypt cost 11 buys you several orders of magnitude more resistance against offline cracking.

App tokens are not human passwords. With 256 bits of cryptographic randomness, there is no dictionary to attack. An offline attacker who steals the hash database cannot feasibly guess the token regardless of hashing speed. The slowness of bcrypt provides zero security benefit here and imposes a real latency cost.

**Proposed: HMAC-SHA256 with a server secret**

```
stored_hash = hex( HMAC-SHA256(token, server_secret) )
```

- **O(1) lookup**: hash the incoming token, look up `stored_hash` directly in the map/index.
- **No iteration**: no need to try each stored token against the presented credential.
- **Server secret**: the HMAC key means an attacker who steals the hash database still cannot verify tokens offline without also compromising the server secret.
- **Standard practice**: this is exactly how GitHub personal access tokens, Stripe API keys, and Slack bot tokens are stored.

Validation becomes:
```go
func (mgr *hmacManager) GetAppPassword(ctx context.Context, userID *userpb.UserId, token string) (*apppb.AppPassword, error) {
    h := computeHMAC(token, mgr.config.ServerSecret) // fast, ~microseconds
    pw, ok := mgr.passwords[userID.String()][h]
    if !ok {
        return nil, errtypes.NotFound("token not found")
    }
    if isExpired(pw) {
        return nil, errtypes.NotFound("token expired")
    }
    pw.Utime = now()
    // save ...
    return pw, nil
}

func computeHMAC(token, secret string) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(token))
    return hex.EncodeToString(mac.Sum(nil))
}
```

**Backwards compatibility note:** The existing JSON files store bcrypt hashes as keys. A migration path is needed: on first load, detect bcrypt-format hashes (they start with `$2a$`) and prompt for re-generation, or keep a bcrypt fallback until all tokens are rotated. This is a deployment concern, not an architectural one.

---

### 13.4 What Is Stored Per Token

The `AppPassword` proto (from `cs3/auth/applications/v1beta1/resources.proto`) currently has:

| Field | Type | Notes |
|---|---|---|
| `password` | `string` | In storage: HMAC hash (proposed) or bcrypt hash (current). In create response: the plaintext token (returned once). In list response: should be a stable opaque `token_id` (see §13.5). |
| `token_scope` | `map[string]*Scope` | Authorization scope for tokens issued from this credential. |
| `label` | `string` | Human-readable device name. |
| `user` | `*UserId` | Owner's user ID. |
| `expiration` | `*Timestamp` | Optional; zero means non-expiring. |
| `ctime` | `*Timestamp` | Creation time. |
| `utime` | `*Timestamp` | Last-used time (updated on every successful authentication). |

**Fields missing from the proto that we need:**

| Field | Purpose | Where to add |
|---|---|---|
| `token_id` | Stable opaque identifier for management operations (see §13.5) | Proposed cs3apis addition |
| `created_by_ua` | User-Agent at token creation (device identification) | Proposed cs3apis addition |
| `created_by_ip` | IP address at token creation (audit trail) | Proposed cs3apis addition |

Until the cs3apis proto is updated, these can be stored in the backend (JSON/SQL) and exposed via a Reva-specific management HTTP API (§15). The `token_id` can also be smuggled through the `Password` field in list responses as a stopgap (see §13.5).

---

### 13.5 The Token ID Problem

**The problem with the current design:**

The current backend uses the bcrypt hash as both the storage key and the revocation handle. `InvalidateAppPassword(ctx, password)` takes what is effectively the bcrypt hash string. This leaks implementation details into the API and creates a circular dependency: the management UI must store the hash (returned by `ListAppPasswords`) to later revoke the token.

**The stable token ID:**

We introduce a `token_id`: a short random identifier generated at token creation, stored alongside the HMAC hash, and returned in all list/get responses.

```go
type StoredToken struct {
    TokenID     string            // e.g. "a3f7c1b2" — 8 random hex bytes (64 bits)
    HMACHash    string            // computeHMAC(plaintext, serverSecret)
    UserID      string
    Label       string
    TokenScope  map[string]*Scope
    Expiration  *Timestamp
    CreatedAt   *Timestamp
    LastUsedAt  *Timestamp
    CreatedByUA string
    CreatedByIP string
}
```

The `token_id` is generated as:
```go
b := make([]byte, 8)
rand.Read(b)
tokenID = hex.EncodeToString(b) // "a3f7c1b2d4e5f601"
```

**Mapping to the existing proto:**

Until `token_id` is added to the CS3 proto, the `Password` field in list/get responses carries the `token_id` (not the hash). The management UI can then revoke by passing this `token_id` to `InvalidateAppPassword`. The backend resolves `token_id → HMACHash` to perform the deletion.

This is a deliberate semantic overloading that should be resolved with a cs3apis PR (see §13.Open Questions).

```
Create:  returns {token_id, plaintext_token, ...}  (plaintext_token: shown once to user)
List:    returns [{token_id, label, ctime, utime, expiration, ...}, ...]  (no plaintext_token, no hash)
Revoke:  InvalidateAppPassword(token_id)
Auth:    GetAppPassword(userID, plaintext_token)  → looks up by HMAC hash
```

---

### 13.6 Storage Backends

**Option A: JSON file (current)**

```json
{
  "alice@cern.ch": {
    "a3f7c1b2": {
      "token_id": "a3f7c1b2",
      "hmac_hash": "3d5f...",
      "label": "Desktop (Linux)",
      "token_scope": {"user": {...}},
      "expiration": null,
      "ctime": 1748476800,
      "utime": 1748563200,
      "created_by_ua": "Nextcloud-desktop/3.14 (Linux)",
      "created_by_ip": "10.0.0.5"
    }
  }
}
```

- **Pros**: Zero new dependencies; already implemented; sufficient for single-instance deployments.
- **Cons**: File I/O on every write (including every `utime` update on auth); global mutex means auth for any user blocks all others; no atomic multi-record operations; not suitable for clustered deployments; `utime` updates on every request create high write amplification (consider throttling: only update if last update was >1 minute ago).

**Option B: SQL (PostgreSQL / SQLite)**

Schema:
```sql
CREATE TABLE app_tokens (
    token_id        TEXT        PRIMARY KEY,
    hmac_hash       TEXT        NOT NULL UNIQUE,
    user_id         TEXT        NOT NULL,
    label           TEXT        NOT NULL DEFAULT '',
    token_scope     JSONB       NOT NULL DEFAULT '{}',
    expiration      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at    TIMESTAMPTZ,
    created_by_ua   TEXT,
    created_by_ip   INET,

    INDEX idx_hmac_hash (hmac_hash),      -- used by every auth request
    INDEX idx_user_id   (user_id)         -- used by list and revoke-all
);
```

- **Pros**: O(1) lookup via index on `hmac_hash`; multi-instance safe; `last_used_at` can be updated without global lock; supports `DELETE WHERE user_id = ?` for revoke-all; survives restarts natively.
- **Cons**: New dependency; migration management; PostgreSQL may be overkill for small deployments (SQLite would work for single-instance).
- **Recommendation**: Implement as a second backend (`pkg/appauth/manager/sql/sql.go`) after the JSON manager is proven. The pluggable interface makes this a drop-in swap.

**Option C: Delegate to external system**

For deployments where Reva sits in front of Nextcloud or another system that already manages app tokens, the `appauth.Manager` interface can be implemented to call the upstream system.
- **Pros**: Single source of truth; no duplication.
- **Cons**: Adds latency; coupling to external system availability.

**Write-amplification mitigation for JSON and SQL:**

Updating `last_used_at` on every request causes a write on every authentication. Solutions:
1. **Throttled writes**: only update if `now() - last_used_at > 60s`. Trades accuracy for write volume.
2. **Async writes**: write `last_used_at` updates to a channel; a background goroutine flushes them every N seconds.
3. **Accept stale**: for audit purposes, `last_used_at` accurate to the nearest minute is usually sufficient.

The JSON backend should adopt throttled writes immediately to avoid lock contention under load.

---

### 13.7 Revocation

**By token_id (normal case):**

```
DELETE /ocs/v2.php/core/apppassword
InvalidateAppPassword(ctx, tokenID)
```

The handler authenticates the request (so we know who is making the request), then calls `InvalidateAppPassword`. If the token being deleted belongs to the authenticated user, the deletion proceeds. This prevents a user from revoking someone else's token.

**By own session (logout flow):**

The Nextcloud sync client calls `DELETE /ocs/v2.php/core/apppassword` using its app token as the Basic Auth password. The handler extracts the token from the Basic Auth header, resolves the `token_id` from the `GetAppPassword` response, and revokes it. This self-revocation pattern is important: the client must be able to clean up after itself on logout without additional state.

**Revoke all (emergency / account suspension):**

```go
// New method to add to appauth.Manager
InvalidateAllAppPasswords(ctx context.Context) error
```

This deletes all tokens for the user in context. Useful for:
- User-initiated "sign out all devices" action.
- Admin-initiated account suspension.
- Automatic cleanup when a user account is deleted.

The JSON backend implementation iterates and deletes; the SQL backend issues `DELETE WHERE user_id = ?`.

**Expiry-based (passive revocation):**

Tokens with a non-zero `expiration` are rejected silently in `GetAppPassword` when the expiry has passed. No active deletion is needed; a periodic cleanup goroutine can garbage-collect expired records.

```go
if pw.Expiration != nil && pw.Expiration.Seconds != 0 && time.Now().Unix() > int64(pw.Expiration.Seconds) {
    return nil, errtypes.NotFound("token expired")
}
```

**Summary of revocation paths:**

| Trigger | Mechanism | Who initiates |
|---|---|---|
| User logout on device | DELETE `/ocs/v2.php/core/apppassword` (self-revocation) | Sync client |
| User revokes via web UI | `InvalidateAppPassword(tokenID)` | User (via management UI) |
| Admin suspends account | `InvalidateAllAppPasswords()` | Admin |
| Token reaches expiry | Passive check in `GetAppPassword` | System (automatic) |
| Periodic cleanup | Garbage collect expired records | Background goroutine |

---

## 14. Management Interface

### 14.1 Existing CS3 API

The `ApplicationsAPI` gRPC service (`cs3/auth/applications/v1beta1`) already provides:

```protobuf
service ApplicationsAPI {
    rpc GenerateAppPassword(GenerateAppPasswordRequest) returns (GenerateAppPasswordResponse);
    rpc ListAppPasswords(ListAppPasswordsRequest) returns (ListAppPasswordsResponse);
    rpc InvalidateAppPassword(InvalidateAppPasswordRequest) returns (InvalidateAppPasswordResponse);
    rpc GetAppPassword(GetAppPasswordRequest) returns (GetAppPasswordResponse);  // internal auth use
}
```

This API is sufficient for basic CRUD but has gaps (see §14.2).

### 14.2 Gaps in the Existing API

| Gap | Impact |
|---|---|
| No `token_id` field | Revocation requires passing the hash (or overloaded `Password` field); exposes internal detail |
| No `created_by_ua` / `created_by_ip` | Management UI cannot show "MacBook, last used from 10.0.0.5" |
| No `InvalidateAllAppPasswords` | Cannot do "sign out all devices" cleanly |
| No `UpdateAppPassword` | Cannot rename a token (e.g., user wants to change device label) |
| No pagination in `ListAppPasswords` | Could be problematic for users with many tokens |
| `GetAppPassword` is the auth path | It updates `utime` on every call; management "get" and auth "get" are conflated |

### 14.3 Proposed HTTP Management API

While the gRPC API evolves (requires cs3apis PRs), we expose a Reva-native HTTP management API under `/ocs/v2.php/apps/reva/tokens` (or a non-OCS path). This API is consumed by the Reva web UI or any management tool.

All endpoints require authentication (Bearer or Basic Auth with any valid credential for the user).

**List tokens:**
```
GET /ocs/v2.php/apps/reva/tokens
```

Response:
```json
{
  "tokens": [
    {
      "token_id": "a3f7c1b2",
      "label": "Desktop (Linux)",
      "created_at": "2026-01-15T10:30:00Z",
      "last_used_at": "2026-05-28T14:22:10Z",
      "expiration": null,
      "created_by_ua": "Nextcloud-desktop/3.14 (Linux)",
      "created_by_ip": "10.0.0.5",
      "scopes": ["user"]
    }
  ]
}
```

Note: the plaintext token is **never** returned by the list endpoint. It is only returned once, at creation.

**Create a token (manual, without Login Flow):**
```
POST /ocs/v2.php/apps/reva/tokens
Content-Type: application/json

{
  "label": "My new device",
  "expiration": "2027-01-01T00:00:00Z"   // optional
}
```

Response `201 Created`:
```json
{
  "token_id": "b4e8d2a1",
  "token": "rvat_X7kLmN2pQr8sT1uV4wYzA5bC6dE9fG0h",   // shown once
  "label": "My new device",
  "created_at": "2026-05-29T09:00:00Z"
}
```

**Revoke a single token:**
```
DELETE /ocs/v2.php/apps/reva/tokens/{token_id}
```

Response `200 OK`.

**Revoke all tokens (sign out all devices):**
```
DELETE /ocs/v2.php/apps/reva/tokens
```

Response `200 OK` with count of revoked tokens.

**Rename a token:**
```
PATCH /ocs/v2.php/apps/reva/tokens/{token_id}
Content-Type: application/json

{ "label": "New label" }
```

Response `200 OK`.

### 14.4 Activity Tracking

**What we track per token:**

| Field | Granularity | Notes |
|---|---|---|
| `created_at` | Exact | Set once at creation |
| `last_used_at` | Throttled (±60s accuracy) | Updated on auth; throttled to avoid write amplification |
| `created_by_ua` | Set once | User-Agent at token creation |
| `created_by_ip` | Set once | IP at token creation |

**What we do not track (and why):**

A full per-request audit log (every WebDAV request from each device) is out of scope for the token storage layer — that belongs in a dedicated audit/logging service. The app token management UI shows _last activity_ (last-used timestamp), not a full request history.

**Recommended UI display:**

```
Token          Label                Created        Last used        Status
──────────────────────────────────────────────────────────────────────────
a3f7c1b2      Desktop (Linux)      Jan 15, 2026   3 hours ago      Active
9e2d4c7a      iPhone               Mar 2, 2026    Yesterday        Active
f1a5b8e3      Old laptop           Dec 1, 2025    6 months ago     Active  ⚠ Unused
```

A "stale token" warning (e.g., unused for >90 days) helps users identify and revoke forgotten tokens. This is purely a UI concern; the backend just exposes `last_used_at`.

### 14.5 Proto Evolution Plan

The gaps in the CS3 API should be addressed with a PR to [cs3org/cs3apis](https://github.com/cs3org/cs3apis):

```protobuf
message AppPassword {
    // existing fields ...
    string password = 1;
    map<string, Scope> token_scope = 2;
    string label = 3;
    cs3.identity.user.v1beta1.UserId user = 4;
    cs3.types.v1beta1.Timestamp expiration = 5;
    cs3.types.v1beta1.Timestamp ctime = 6;
    cs3.types.v1beta1.Timestamp utime = 7;

    // NEW fields (proposed)
    string token_id = 8;          // stable opaque ID for management operations
    string created_by_ua = 9;     // User-Agent at creation
    string created_by_ip = 10;    // IP address at creation (string, not INET, for proto compat)
}

service ApplicationsAPI {
    // existing RPCs ...
    rpc InvalidateAllAppPasswords(InvalidateAllAppPasswordsRequest) 
        returns (InvalidateAllAppPasswordsResponse);
    rpc UpdateAppPassword(UpdateAppPasswordRequest)
        returns (UpdateAppPasswordResponse);
}
```

Until this PR is merged, the Reva HTTP management API (§14.3) will use a separate storage layer that tracks these extra fields without surfacing them through the CS3 proto.

---

## 15. Open Questions

1. **Multi-instance deployments**: The in-memory session store does not work if multiple Reva instances are behind a load balancer without sticky sessions. Should we include a Redis backend in the initial implementation, or wait for a concrete need?

2. **`token_id` in CS3 proto**: The `AppPassword` proto lacks a stable `token_id` field; we are currently overloading the `Password` field in list responses with an opaque ID. A PR to cs3org/cs3apis should add `token_id`, `created_by_ua`, and `created_by_ip` fields, plus `InvalidateAllAppPasswords` and `UpdateAppPassword` RPCs. This should be filed before implementation starts so the code can target the new proto from day one.

3. **Hashing migration**: The JSON backend currently stores bcrypt hashes as keys. Switching to HMAC-SHA256 is a breaking change for existing token files. Migration options: (a) detect the format at load time and keep a bcrypt fallback path; (b) force all users to re-generate tokens on upgrade (add to release notes); (c) provide a one-off migration tool. Option (a) is safest for production deployments.

4. **Scope negotiation**: Should the Login Flow v2 initiation request allow a client to request scopes? This would require an extension to the protocol. We should reach out to the Nextcloud team to understand if this is on their roadmap.

5. **Token format compatibility**: The Nextcloud client validates the app password by attempting a `PROPFIND` against the WebDAV root. Does Reva's existing `appauth` auth manager handle this correctly for all configured auth backends? A compatibility test should be added.

6. **Login Flow v2 with LDAP backend**: The form handler authenticates via the configured credential strategy. If the deployment uses LDAP, the submitted password goes to the LDAP manager. However, the LDAP password is never stored in Reva — the app token is generated separately. This is the intended design, but should be validated with an LDAP integration test.

7. **`last_used_at` write amplification**: The JSON backend acquires a global mutex on every auth request to update `utime`. Under concurrent load this becomes a bottleneck. The document recommends throttled writes (update at most once per 60 seconds), but the exact threshold and implementation strategy should be decided before the SQL backend is built to avoid encoding the wrong behavior.
