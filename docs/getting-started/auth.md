# Authentication & Authorization

This page covers how Team Health Check authenticates users and enforces role-based access control (RBAC).

---

## Authentication Methods

Team Health Check supports two authentication methods simultaneously. Both coexist — enabling one does not disable the other.

| Method | Description | When to use |
|--------|-------------|------------|
| **Username / password** | Credentials stored in `users.password_hash` (bcrypt) | Default; always available |
| **SSO / OIDC (PKCE)** | Authorization Code flow with PKCE; no client secret required | Enabled by setting OAuth env vars |

---

## Username / Password Flow

```
1. User submits credentials → POST /api/v1/auth/login
2. auth_handler.go calls UserRepository.FindByUsername()
3. bcrypt.CompareHashAndPassword(storedHash, submittedPassword)
4. On match: backend issues a JWT access token + refresh token
5. Frontend stores accessToken/refreshToken in localStorage, and the full
   user object (URL-encoded JSON) in a `user` cookie (js-cookie, 7-day
   expiry — max-age=604800)
6. Every API request goes through `authenticatedFetch()`, which attaches
   `Authorization: Bearer <accessToken>` and, on a 401, transparently calls
   POST /api/v1/auth/refresh and retries the request once
7. middleware.ts reads the `user` cookie on every page load for route
   protection; missing/invalid cookie → redirect to /login
```

**Password hashing:** bcrypt with `bcrypt.DefaultCost` (cost factor 10, Go `golang.org/x/crypto/bcrypt`). Passwords are never stored in plain text.

**Demo passwords:** All demo accounts use the string `"demo"` as the password (hashed). Admin uses `"admin"`. The `admin` account is seeded by migration `000007` (runs in every environment); the remaining demo accounts (`vp`, `director1`, `manager1`, `teamlead1`, `demo`, etc.) are created by `SeedDemoData()` at application startup, which only runs when `APP_ENV=demo`. Never use demo credentials in production.

### Token & Session Storage

| Item | Storage | Format / Key | Expiry | Purpose |
|------|---------|--------------|--------|---------|
| **Access Token** | `localStorage` | Key: `accessToken` | JWT expiry (from backend) | API requests authorization |
| **Refresh Token** | `localStorage` | Key: `refreshToken` | Long-lived | Refreshing access token via `POST /api/v1/auth/refresh` |
| **User Session Cookie** | Cookie | Key: `user` (URL-encoded JSON) | 7 days (`max-age=604800`) | Middleware route protection |

---

## SSO / OIDC Flow (Authorization Code + PKCE)

Team Health Check supports any OIDC-compliant provider: Keycloak, Okta, Auth0, Google, Azure AD, etc.

```
1. Frontend fetches SSO config dynamically from GET /api/v1/config
2. User clicks "Sign in with SSO" on /login
3. Frontend generates PKCE code_verifier + code_challenge
4. Frontend redirects to provider's /authorize endpoint:
      ?client_id=...
      &redirect_uri=.../auth/callback
      &response_type=code
      &scope=openid email profile
      &code_challenge=...
      &code_challenge_method=S256
5. User authenticates at provider
6. Provider redirects to /auth/callback?code=...
7. Frontend calls POST /api/v1/auth/sso/callback
      { code, code_verifier, redirect_uri }
8. Backend calls provider's token endpoint to exchange code → tokens
9. Backend extracts `email` from ID token claims
10. Backend looks up user by email in users table
11. If found: return JWT access token, refresh token, and user info
12. If not found: return 401 "SSO account not provisioned"
    (Team Health Check does not auto-create users from SSO logins)
```

### Configure SSO

SSO configuration is configured on the **backend** (`backend/.env`):

```env
OAUTH_CLIENT_ID=your-client-id
OAUTH_AUTHORIZE_URL=https://your-provider.com/oauth/authorize
OAUTH_TOKEN_URL=https://your-provider.com/oauth/token
OAUTH_REDIRECT_URI=http://localhost:3000/auth/callback
OAUTH_SCOPES=openid email profile
```

The frontend fetches these configuration parameters at runtime via `GET /api/v1/config`. If `OAUTH_CLIENT_ID` is not set on the backend, the "Sign in with SSO" button is automatically hidden.

### Provider setup quick reference

| Setting | Value |
|---------|-------|
| Application type | Single Page App (SPA) / Public client |
| Client secret | Not required |
| Allowed redirect URI | `http://localhost:3000/auth/callback` (or your domain) |
| Required scopes | `openid email profile` |
| Token claim needed | `email` in ID token |

---

## Role-Based Access Control (RBAC)

### Hierarchy Levels

Permissions are tied to `hierarchy_levels`, not to individual users. Each level has boolean flags (see `DEFAULT_ORG_CONFIG` in `frontend/lib/org-config.ts`):

| Permission flag | VP (L1) | Director (L2) | Manager (L3) | Team Lead (L4) | Team Member (L5) |
|----------------|---------|--------------|-------------|---------------|-----------------|
| `canViewAllTeams` | true | true | false | false | false |
| `canEditTeams` | true | true | true | false | false |
| `canManageUsers` | true | true | false | false | false |
| `canConfigureSystem` | true | false | false | false | false |
| `canViewReports` | true | true | true | true | false |
| `canExportData` | true | true | true | true | false |

These defaults can be modified via the Admin panel (Hierarchy Levels tab) without a code change.

`isAdmin` is a separate boolean on the `User` record (`hierarchyLevelId === 'level-admin'`) — it is not a `hierarchyLevels` entry and is not part of this permission table. Admin users bypass hierarchy-level permission checks entirely.

`canTakeSurvey` is likewise **not** a hierarchy-level permission — it's a per-user boolean (`user.canTakeSurvey`) that `frontend/middleware.ts` checks directly to gate access to `/survey`.

### Route Protection

`frontend/middleware.ts` enforces routes based on the `user` cookie (simplified):

```typescript
// Simplified logic — see frontend/middleware.ts for the real implementation
const userCookie = request.cookies.get('user');
if (!userCookie && !isPublicPath) return redirect('/login');

const user = JSON.parse(userCookie.value);

// On /login, redirect to the dashboard for the user's hierarchy level
// (admin -> /admin, L1-L3 -> /manager, L4 -> /dashboard, L5 -> /home)

// On /survey, redirect away unless the per-user flag allows it
if (path === '/survey' && user.canTakeSurvey !== true) {
  return redirect(/* role-appropriate dashboard */);
}
```

### Team Access Control

A user can access a team's data if **any** of the following is true:

1. `canViewAllTeams` is true for their hierarchy level (VP, Director; Admin bypasses this check entirely)
2. The user is a member of the team (`team.members`)
3. The user appears in the team's `supervisorChain`

Logic implemented in `frontend/lib/org-config.ts`:

```typescript
export function canUserAccessTeam(user: User, team: Team): boolean {
  const permissions = getUserPermissions(user);
  if (permissions.canViewAllTeams) return true;
  if (team.members.includes(user.id)) return true;
  return team.supervisorChain.some(s => s.userId === user.id);
}
```

---

## Password Reset

Team Health Check implements a short-lived (1-hour), single-use token flow:

```
1. User submits email on /forgot-password
2. Backend creates a password_reset_tokens row (token_hash stored, not plaintext)
3. Email sent with reset link (token in URL)
4. User clicks link → token verified → password updated → token marked used
```

Token table: `password_reset_tokens` — see [DB Schema](../architecture/db-schema.md#password_reset_tokens).

---

## Backend API Authorization

The JWT/RBAC checks above are frontend route protection (which page loads). The Go API also enforces authorization independently at the handler level via middleware in `backend/interfaces/middleware/jwt_auth.go`:

| Middleware | Enforces |
|------------|----------|
| `JWTAuthMiddleware` | Valid, non-expired access token required |
| `OptionalJWTAuthMiddleware` | Attaches user context if a token is present, but doesn't require one |
| `AdminOnlyMiddleware` | Caller's hierarchy level is `level-1` or `level-admin` (hardcoded level IDs, not the `canConfigureSystem` permission flag) |
| `ManagerOrAboveMiddleware` | Caller's hierarchy level is `level-1`, `level-2`, `level-3`, or `level-admin` (hardcoded level IDs) |
| `SameUserOrManagerMiddleware` | Caller is the target user or their manager |
| `TeamMembershipMiddleware` | Caller is a member of, or supervisor over, the `:teamId` route param |

The full set of public (no-JWT) `/api/v1/*` routes is: `/api/v1/auth/login`, `/api/v1/auth/refresh`, `/api/v1/auth/logout` (`backend/interfaces/api/v1/auth_routes.go`); `/api/v1/auth/sso/callback` and `/api/v1/config` (`backend/interfaces/api/v1/sso_routes.go`); and `/api/v1/auth/forgot-password`, `/api/v1/auth/reset-password` (`backend/interfaces/api/v1/password_reset_handler.go`). Every other `/api/v1/*` route requires a valid JWT.

---

## Known Gaps

- The password reset endpoint (`/forgot-password`) is not yet rate-limited.
- The `user` cookie does not set `HttpOnly` or `Secure` — it's read by client-side JS (`middleware.ts`, `frontend/lib/auth.ts`) and is not intended to hold the JWTs themselves.
