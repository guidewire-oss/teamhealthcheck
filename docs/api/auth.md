# Authentication API

Registered by `SetupAuthRoutes`, `SetupSSORoutes`, and `SetupPasswordResetRoutes`.
Every route on this page is **public** — none of them carries any guard middleware.

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/auth/login` | *public* | Exchange username + password for a token pair |
| POST | `/api/v1/auth/refresh` | *public* | Exchange a refresh token for a new access token |
| POST | `/api/v1/auth/logout` | *public* | Client-side logout acknowledgement |
| POST | `/api/v1/auth/sso/callback` | *public* | Complete the OAuth 2.0 PKCE flow |
| GET | `/api/v1/config` | *public* | Runtime SSO settings and branding |
| POST | `/api/v1/auth/forgot-password` | *public* | Request a password reset email |
| POST | `/api/v1/auth/reset-password` | *public* | Consume a reset token and set a new password |

!!! warning "Login is not rate limited"
    `AuthRateLimiter` (10 requests/minute) is defined in
    `interfaces/api/middleware/validation.go` but is never applied in `main.go`. There is
    no brute-force protection on these endpoints.

---

## POST /api/v1/auth/login

**Request** — `dto.LoginRequest`

| Field | Type | Validation |
|-------|------|-----------|
| `username` | string | required |
| `password` | string | required |

**Response** `200` — `dto.LoginResponse`

```json
{
  "user": {
    "id": "alice",
    "username": "alice",
    "email": "alice@example.com",
    "fullName": "Alice Johnson",
    "hierarchyLevel": "level-5",
    "teamIds": ["platform-squad"],
    "canTakeSurvey": true
  },
  "accessToken": "eyJhbGciOi...",
  "refreshToken": "eyJhbGciOi...",
  "expiresIn": 900
}
```

`teamIds` is the deduplicated union of teams the user belongs to and teams the user
leads; it is never `null`. `canTakeSurvey` comes from the user's hierarchy level
permissions — if that lookup fails, it silently stays `false`.

**Errors**

| Status | `error` | Cause |
|--------|---------|-------|
| 400 | `Username and password are required` | Bind failure |
| 401 | `Invalid username or password` | Unknown username, an SSO-only account attempting password login, or a bcrypt mismatch |
| 500 | `Failed to generate authentication tokens` | Token generation failed |

The three 401 causes are deliberately indistinguishable to the client; they are
separated only in the structured log (`user_not_found`, `sso_user_local_login`,
`incorrect_password`).

---

## POST /api/v1/auth/refresh

**Request** — `dto.RefreshTokenRequest`

| Field | Type | Validation |
|-------|------|-----------|
| `refreshToken` | string | required |

**Response** `200` — `{"accessToken": "...", "expiresIn": 900}`

!!! warning "`expiresIn` is hardcoded"
    The refresh response always reports `900`, regardless of `JWT_ACCESS_EXPIRY`. If you
    configure a different access token lifetime, this field will be wrong.

**Errors**

| Status | `error` | Cause |
|--------|---------|-------|
| 400 | `Refresh token is required` | Bind failure |
| 401 | `Invalid or expired refresh token` | Validation failed (expired and invalid are not distinguished to the client) |
| 401 | `User not found` | The token's subject no longer exists |
| 401 | `Failed to refresh token` | New access token could not be generated |

!!! note "Inconsistent status for generation failure"
    A server-side token generation failure returns `401` here but `500` on login.

---

## POST /api/v1/auth/logout

Takes no body and reads no parameters. Always returns `200 {"message": "Logged out successfully"}`.

The handler optionally reads `user_id` from the context for logging only. Because the
route carries no JWT middleware, an unauthenticated call succeeds and is logged as a
successful logout.

!!! warning "Logout does not invalidate anything"
    There is no token blocklist or server-side session. The endpoint is an
    acknowledgement; the client is responsible for discarding its tokens. A leaked
    refresh token remains valid until it expires.

---

## POST /api/v1/auth/sso/callback

Completes the OAuth 2.0 Authorization Code + PKCE exchange. See
[Authentication & Authorization](../architecture/authentication.md#sso) for the flow.

**Request** — an unexported struct in `sso_handler.go` (note the snake_case field, unlike
the camelCase used everywhere else in the API):

| Field | Type | Validation |
|-------|------|-----------|
| `code` | string | required |
| `code_verifier` | string | required |

**Response** `200` — `dto.LoginResponse`, identical in shape to login.

!!! warning "`canTakeSurvey` is always `false` for SSO logins"
    `SSOHandler` is constructed without an `organization.Repository`, so it cannot
    resolve the permission. Because `frontend/middleware.ts` gates `/survey` on this
    flag, SSO users are redirected away from the survey page.

**Errors**

| Status | `error` |
|--------|---------|
| 503 | `SSO is not configured on this server` (`OAUTH_CLIENT_ID` or `OAUTH_TOKEN_URL` unset) |
| 400 | `code and code_verifier are required` |
| 401 | `Token exchange with OAuth provider failed` |
| 401 | `Could not determine email from SSO token` |
| 401 | `No account found for this email address. Please contact your administrator.` |
| 401 | `This account does not support SSO login. Please use username and password.` |
| 500 | `Failed to generate authentication tokens` |

There is no just-in-time provisioning — the account must already exist with
`auth_type = 'sso'`.

---

## GET /api/v1/config

Public runtime configuration, so that the login page can render the SSO button and
company branding without baking values in at build time. Defined as an inline closure in
`sso_routes.go`, not a handler method.

**Response** `200`

```json
{
  "appEnv": "production",
  "companyName": "My Company",
  "logoURL": null,
  "sso": {
    "clientId": "...",
    "authorizeUrl": "https://idp.example.com/authorize",
    "redirectUri": "https://teams360.example.com/auth/callback",
    "scopes": "openid email profile"
  }
}
```

`sso` is `null` when `OAUTH_CLIENT_ID` is unset. `appEnv` defaults to `production` here
(note: `main.go` defaults the same variable to `dev`). `companyName` and `logoURL` come
from `app_settings`; if that read fails, `companyName` falls back to `My Company` and
`logoURL` to `null`. `scopes` defaults to `openid email profile`.

The frontend uses `appEnv === "demo"` to decide whether to show the demo credentials
panel on the login page.

This endpoint has no error responses — repository failures fall back to defaults.

---

## POST /api/v1/auth/forgot-password

**Request** — `dto.ForgotPasswordRequest`: `{"email": "..."}`

**Response** `200`

```json
{"message": "If your email is registered, you will receive a password reset link shortly"}
```

The same message is returned whether or not the address exists, to prevent enumeration.

**Errors**

| Status | `error` |
|--------|---------|
| 400 | `Email is required` (bind failure) |
| 400 | `Invalid email format` |
| 500 | `Failed to process request` |

---

## POST /api/v1/auth/reset-password

**Request** — `dto.ResetPasswordRequest`

| Field | Type |
|-------|------|
| `token` | string |
| `newPassword` | string |

**Response** `200` — `{"message": "Password has been reset successfully"}`

**Errors**

| Status | `error` |
|--------|---------|
| 400 | `Token and new password are required` (bind failure) |
| 400 | `Token is required` |
| 400 | `New password is required` |
| 400 | `Password must be at least 8 characters` |
| 401 | `Invalid or expired reset token` |
| 500 | `Failed to reset password` |

!!! warning "No UI reaches these two endpoints"
    The reset flow is fully implemented server-side — service, token table, email
    template — but nothing in the frontend links to it. There is no "Forgot password?"
    link on the login page. The endpoints are usable only by calling the API directly.
