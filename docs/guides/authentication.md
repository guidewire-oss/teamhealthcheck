# Signing In

This guide covers the sign-in workflows a user actually encounters. For the mechanism —
JWT issuance, claim contents, middleware guards — see [Authentication &
Authorization](../architecture/authentication.md).

## Username and password

The login page (`frontend/app/login/page.tsx`) asks for a username and a password and
posts them to `POST /api/v1/auth/login`. On failure it shows either the server's `error`
message or `Invalid username or password`; if the request never reaches the server you get
`Unable to connect. Please try again later.`

On success the browser stores three things:

| What | Where | Lifetime |
|------|-------|----------|
| `accessToken` | `localStorage` | Per the JWT's own expiry |
| `refreshToken` | `localStorage` | Per the JWT's own expiry |
| `user` (JSON, URL-encoded) | Cookie named `user`, `path=/` | 7 days (`max-age=604800`) |

You are then redirected by hierarchy level: `level-admin` → `/admin`, `level-1`/`level-2`/
`level-3` → `/manager`, `level-4` → `/dashboard`, anything else → `/home`.

!!! note "The cookie holds the user, not the token"
    Only the user object lives in a cookie. Tokens stay in `localStorage`, which is why
    `frontend/middleware.ts` can route by role but cannot verify a session — it reads
    `user` and trusts it. Real enforcement happens server-side on every API call. The
    practical consequence: an expired token does not clear the cookie, so you stay
    "logged in" visually until the first API call fails.

### Session expiry

`authenticatedFetch` in `frontend/lib/auth.ts` transparently retries with the refresh
token. If the refresh also fails it sends you to `/login?expired=true`, which renders the
banner *"Your session has expired. Please log in again."*

### Demo credentials panel

When `GET /api/v1/config` reports `appEnv: "demo"`, the login page shows a **Demo Login
Credentials** panel listing `vp/demo`, `director1/demo`, `manager1/demo`, `teamlead1/demo`,
`demo/demo` and `admin/admin`. Outside a demo deployment the panel is absent — and so are
most of those accounts. See the demo-credentials note on the [home page](../index.md).

## Single sign-on

The **Sign in with SSO** button appears only when `GET /api/v1/config` returns a non-null
`sso` object, which happens only when the `OAUTH_CLIENT_ID` environment variable is set
(`backend/interfaces/api/v1/sso_routes.go:37`). With no OAuth configuration the button is
simply not rendered — there is nothing to enable in the admin UI.

The flow is a standard authorization-code exchange with PKCE:

1. `startSSOFlow` (`frontend/lib/sso.ts`) generates a PKCE verifier and a `state` value,
   stashes both in `sessionStorage`, and redirects to the identity provider's
   `authorizeUrl` with `code_challenge_method=S256`.
2. The provider redirects back to `/auth/callback`, which validates `state`, recovers the
   verifier, and posts `{ code, code_verifier }` to `POST /api/v1/auth/sso/callback`.
3. The backend exchanges the code at `OAUTH_TOKEN_URL`, extracts the email from the
   returned ID token, looks up the user, and issues Team360's own JWT pair. From there the
   redirect rules are identical to password login.

**SSO does not create accounts.** The user must already exist *and* have `auth_type = 'sso'`:

- Unknown email → `No account found for this email address. Please contact your administrator.`
- Known email with `auth_type = 'local'` → `This account does not support SSO login. Please use username and password.`

An administrator sets `auth_type` on the user record — see
[Administration](administration.md).

!!! warning "The SSO login response omits `canTakeSurvey`"
    `sso_handler.go:111-118` builds its user DTO without the `canTakeSurvey` field, while
    the password path populates it from the hierarchy level
    (`auth_handler.go:156-173`). Because `frontend/middleware.ts` gates `/survey` on
    `user.canTakeSurvey === true`, a user who signs in via SSO is redirected away from the
    survey page even when their hierarchy level permits it. Signing in with a password
    works around it.

!!! warning "ID token signatures are not verified"
    `extractEmailFromJWT` uses `jwt.NewParser().ParseUnverified`
    (`sso_handler.go:158-172`), by explicit comment. The token is trusted because it came
    back over TLS directly from the configured `OAUTH_TOKEN_URL`, not because it was
    validated. Anyone hardening this deployment should treat that as a gap.

## Password reset

Two public endpoints exist:

| Method | Path | Body |
|--------|------|------|
| `POST` | `/api/v1/auth/forgot-password` | `{ "email": "..." }` |
| `POST` | `/api/v1/auth/reset-password` | `{ "token": "...", "newPassword": "..." }` |

`forgot-password` always returns 200 with *"If your email is registered, you will receive a
password reset link shortly"*, whether or not the address exists — deliberate, to prevent
account enumeration. The token is 32 random bytes, stored only as a bcrypt hash, valid for
**one hour**, and single-use. Passwords must be at least 8 characters
(`Password must be at least 8 characters`). Users with `auth_type = 'sso'` are silently
skipped: no token is created and no email is sent.

!!! warning "There is no password-reset UI"
    No `/forgot-password` or `/reset-password` page exists under `frontend/app/`, and the
    login page has no "forgot password?" link. The endpoints are reachable only by calling
    the API directly. In practice, resets are done by an administrator typing a new
    password into the user edit form — see [Administration](administration.md).

!!! warning "With no mail service configured, the reset token goes nowhere"
    `backend/cmd/api/main.go:167-187` picks AWS SES if configured, else SMTP, else leaves
    the sender `nil`. `password_reset_service.go:135-141` guards the send on
    `emailService != nil`, so with neither configured the token is created and stored but
    never delivered — and the HTTP handler discards the plaintext token rather than
    returning it. The only trace is the log line `password_reset_token_created`. See
    [Configuration](../operations/configuration.md) for the `AWS_SES_*` and `SMTP_*`
    variables.

## Route protection

`frontend/middleware.ts` runs on every request except `/api/*`, `/_next/static`,
`/_next/image` and `/favicon.ico`. Its rules, in order:

1. **No `user` cookie** and not on `/` or `/login` → redirect to `/login`.
2. **Already signed in and on `/login`** → redirect to the landing route for your level.
3. **Requesting `/survey`** without `user.canTakeSurvey === true` → redirect to your
   landing route.

!!! note "`/auth/callback` is not on the public list"
    The SSO callback route is not exempted from rule 1, so an unauthenticated hit is
    redirected to `/login`. It works in practice because the callback page completes its
    token exchange and sets the `user` cookie during client-side rendering.

This is convenience routing only. Every protected endpoint independently validates the JWT
server-side; see [Authentication & Authorization](../architecture/authentication.md) and
the [Auth API reference](../api/auth.md).

## Signing out

Every dashboard has a **Logout** button. It clears both tokens from `localStorage`, expires
the `user` cookie and returns you to `/login`. There is no server-side token revocation —
an already-issued access token remains valid until it expires.
