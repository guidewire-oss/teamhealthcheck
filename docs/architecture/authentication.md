# Authentication & Authorization

Team360 authenticates with JWT bearer tokens issued by the Go API, and authorizes with a
set of Gin middleware keyed on the user's hierarchy level.

## Token model

`backend/application/services/jwt_service.go` issues an access/refresh token pair.

| Setting | Environment variable | Default |
|---------|---------------------|---------|
| Signing secret | `JWT_SECRET` | **randomly generated at boot** |
| Access token lifetime | `JWT_ACCESS_EXPIRY` | `15m` |
| Refresh token lifetime | `JWT_REFRESH_EXPIRY` | `168h` (7 days) |
| Issuer | `JWT_ISSUER` | `teams360` |

!!! danger "`JWT_SECRET` must be set in any real deployment"
    When `JWT_SECRET` is unset, `NewJWTService` generates a random 32-byte secret at
    process start and logs a security warning. Consequences: every restart invalidates
    all outstanding tokens, and multiple replicas cannot validate each other's tokens.
    `JWT_SECRET` is **not** listed in `.env.example` — see
    [Configuration](../operations/configuration.md).

The token claims (`services.TokenClaims`) carry `UserID`, `Username`, `Email`,
`HierarchyLevel`, and `TeamIDs`. `JWTAuthMiddleware` copies each of these onto the Gin
context as `userID`, `username`, `email`, `hierarchyLevel`, `teamIDs`, and the whole
claims struct as `claims`.

!!! note "`teamIDs` is baked into the token"
    Team membership is read from the JWT, not from the database, on every guarded
    request. A user added to or removed from a team will not see the change reflected in
    `TeamMembershipMiddleware` until their access token is refreshed.

## Guard middleware

All guards live in `backend/interfaces/middleware/jwt_auth.go` and must run **after**
`JWTAuthMiddleware`.

### `JWTAuthMiddleware`

Requires an `Authorization: Bearer <token>` header. Failure modes, all returning
`401` with `{"error": ...}`:

| Condition | Message |
|-----------|---------|
| No `Authorization` header | `Authorization header is required` |
| Header is not `Bearer <token>` | `Authorization header must be Bearer token` |
| `services.ErrExpiredToken` | `Token has expired` |
| `services.ErrInvalidToken` | `Invalid token` |
| Any other validation error | `Authentication failed` |

Each failure emits a structured `log.Auth("token_validation")` event with a machine
`Reason` (`missing_authorization_header`, `invalid_authorization_format`,
`access_token_expired`, `access_token_invalid`, `token_validation_error`).

`OptionalJWTAuthMiddleware` exists in the same file and validates a token if present
without requiring one. It is not used by any registered route.

### `AdminOnlyMiddleware`

Permits `level-1` and `level-admin`. Otherwise `403 Access denied: admin privileges required`
(or `403 Access denied: unable to determine user role` if the level is missing from the
context).

!!! warning "`level-1` is VP, not Admin"
    The seed data assigns `level-1` to **VP** and `level-admin` to Admin, but
    `AdminOnlyMiddleware` accepts both. Every VP therefore has full administrative access
    to `/api/v1/admin/*`, including user and hierarchy management. The comment in
    `admin_routes.go` describing admin as "level-1" reflects an older hierarchy.

### `ManagerOrAboveMiddleware`

Permits `level-1`, `level-2`, `level-3`, and `level-admin`. Otherwise
`403 Access denied: manager or above privileges required`.

### `SameUserOrManagerMiddleware(paramName)`

Compares the authenticated `userID` against the named path parameter.

- Match → allowed.
- `level-1`, `level-2`, or `level-admin` → allowed for any target user.
- Everything else (including `level-3` managers targeting another manager) →
  `403 Access denied: cannot access other user's data`.

!!! warning "Subordinate relationship is not verified"
    A `TODO` in the source notes that directors and above are allowed through without
    checking that the target user is actually a subordinate. Any director can read any
    manager's dashboard data.

### `TeamMembershipMiddleware(paramName)`

- `level-1`, `level-2`, `level-3`, `level-admin` → allowed for **any** team, with no
  supervisor-chain check.
- Everyone else → the target team ID must appear in the `teamIDs` claim, otherwise
  `403 Access denied: you are not a member of this team`.
- If the path parameter is empty the middleware calls `c.Next()` and defers to the
  handler.

!!! warning "Manager scoping is not enforced at the team level"
    Because managers and above are allowed through unconditionally, any manager can read
    any team's dashboard by ID, not only teams in their own supervisor chain.

## Route-to-guard mapping

| Route group | Guards |
|-------------|--------|
| `POST /api/v1/auth/{login,refresh,logout}` | none (public) |
| `POST /api/v1/auth/{forgot-password,reset-password}` | none (public) |
| `POST /api/v1/auth/sso/callback`, `GET /api/v1/config` | none (public) |
| `GET /health` | none (public) |
| `/api/v1/health-checks`, `/api/v1/health-dimensions`, `/api/v1/assessment-periods`, `/api/v1/teams/:teamId/submission-status` | JWT |
| `GET /api/v1/teams` | JWT |
| `/api/v1/teams/:teamId/{sessions,info}` | JWT + TeamMembership |
| `/api/v1/teams/:teamId/dashboard/*` | JWT + TeamMembership |
| `/api/v1/teams/:teamId/action-items*` | JWT + TeamMembership |
| `/api/v1/managers/:managerId/teams/action-items` | JWT + ManagerOrAbove (+ in-handler `claims.UserID != managerId` → 403) |
| `/api/v1/managers/:managerId/*` (other) | JWT + ManagerOrAbove + SameUserOrManager |
| `/api/v1/users/me` | JWT |
| `/api/v1/users/:userId/survey-history` | JWT + SameUserOrManager |
| `/api/v1/admin/**` | JWT + AdminOnly |

## Local password login

`AuthHandler.Login` (`backend/interfaces/api/v1/auth_handler.go`):

1. Binds `{username, password}`; both are `binding:"required"`.
2. Looks the user up by username.
3. Rejects the login if `AuthType == "sso"` — SSO accounts cannot use password login.
4. Compares the password with `bcrypt.CompareHashAndPassword`.
5. Resolves `canTakeSurvey` from the user's hierarchy level permissions.
6. Computes `teamIds` as the deduplicated union of teams the user is a member of and
   teams the user leads.
7. Issues the token pair.

All three failure branches (unknown user, SSO account, wrong password) return the same
`401 Invalid username or password`, with the distinguishing reason recorded only in the
structured log. This is intentional — it prevents username enumeration.

## SSO

`SSOHandler.Callback` implements the server half of an OAuth 2.0 **Authorization Code
flow with PKCE**. The frontend (`frontend/lib/sso.ts`) generates the verifier and state,
stores them in `sessionStorage`, and redirects to the provider; the callback page posts
`{code, code_verifier}` back to `/api/v1/auth/sso/callback`.

Server configuration comes entirely from environment variables — `OAUTH_CLIENT_ID`,
`OAUTH_TOKEN_URL`, `OAUTH_REDIRECT_URI`, plus `OAUTH_AUTHORIZE_URL` and `OAUTH_SCOPES`
which are echoed to the browser by `GET /api/v1/config`. If `OAUTH_CLIENT_ID` or
`OAUTH_TOKEN_URL` is unset, the callback returns `503 SSO is not configured on this server`
and `/api/v1/config` returns `"sso": null`, which hides the button in the UI.

The handler exchanges the code, extracts `email` from the `id_token` (falling back to the
`access_token`), and requires that a user with that email already exists **and** has
`auth_type = 'sso'`. There is no just-in-time provisioning: accounts must be created by
an administrator first.

!!! danger "Provider token signatures are not verified"
    `extractEmailFromJWT` uses `jwt.NewParser().ParseUnverified`
    (`backend/interfaces/api/v1/sso_handler.go`). The identity provider's token
    signature is never checked. Trust rests entirely on the TLS-protected token-exchange
    response from `OAUTH_TOKEN_URL`.

!!! warning "SSO logins never get `canTakeSurvey`"
    `SSOHandler` has no `organization.Repository`, so the `canTakeSurvey` field of the
    login response is always `false` for SSO users. Because `frontend/middleware.ts`
    gates `/survey` on that flag, SSO users are redirected away from the survey page.

## Password reset

Two public endpoints, backed by `application/services/password_reset_service.go` and the
`password_reset_tokens` table:

- `POST /api/v1/auth/forgot-password` — always returns the same success message
  regardless of whether the email exists, to prevent enumeration. The reset link is sent
  by the configured email sender.
- `POST /api/v1/auth/reset-password` — consumes the token and sets a new password, which
  must be at least 8 characters.

!!! warning "Password reset has no user interface"
    Both endpoints, the service, the token table, and the email template exist and work,
    but no frontend page references them — there is no "Forgot password?" link on the
    login page. The feature is reachable only by calling the API directly.

## Frontend session handling

`frontend/lib/auth.ts` stores the `accessToken` and `refreshToken` in **localStorage**,
and separately writes a non-HttpOnly `user` cookie (`path=/`, `max-age=604800`) holding
the URL-encoded user JSON. The cookie exists purely so that `frontend/middleware.ts` can
make routing decisions at the edge; it carries no credential.

`authenticatedFetch` attaches the bearer token, and on a 401 attempts a single refresh
via `POST /api/v1/auth/refresh` before retrying. If the refresh fails it navigates to
`/login?expired=true`.

`frontend/middleware.ts` treats only `/` and `/login` as public, redirects unauthenticated
requests to `/login`, redirects authenticated users landing on `/login` to their
role's home route, and gates `/survey` on `user.canTakeSurvey`.

!!! warning "Frontend route protection is presentational only"
    Middleware checks only that the `user` cookie parses — it cannot verify a signature,
    because the JWT is in localStorage. It does not restrict `/admin`, `/manager`,
    `/dashboard`, or `/teams/*` by role; `/admin` is guarded only by a client-side
    `useEffect` redirect and `/manager` by nothing at all. Real enforcement is the
    server-side middleware documented above. Additionally, `/auth/callback` is missing
    from the middleware's public path list even though `AuthGuard` treats it as public.
