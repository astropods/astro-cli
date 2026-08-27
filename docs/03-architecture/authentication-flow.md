# Astro Authentication Flow

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-27

This document describes the authentication system used in Astro, including the OAuth flow with WorkOS, session management, and security measures.

## Overview

Astro uses WorkOS AuthKit for authentication, which provides:
- Single Sign-On (SSO) via Google, Microsoft, GitHub, etc.
- Email/password authentication
- Organization and role management

## Authentication Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                              ASTRO AUTHENTICATION FLOW                                   │
└─────────────────────────────────────────────────────────────────────────────────────────┘

                                    ┌─────────────────┐
                                    │   USER/BROWSER  │
                                    └────────┬────────┘
                                             │
     ┌───────────────────────────────────────┼───────────────────────────────────────┐
     │ FRONTEND (React)                      │                                       │
     │                                       ▼                                       │
     │                             ┌─────────────────┐                               │
     │                             │  AuthProvider   │                               │
     │                             │  (React Context)│                               │
     │                             └────────┬────────┘                               │
     │                                      │                                        │
     │              ┌───────────────────────┼───────────────────────┐                │
     │              │                       │                       │                │
     │              ▼                       ▼                       ▼                │
     │    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐          │
     │    │  login()        │    │  checkAuth()    │    │  logout()       │          │
     │    │  → redirect to  │    │  → GET /auth/me │    │  → redirect to  │          │
     │    │  /auth/login    │    │  (credentials:  │    │  /auth/logout   │          │
     │    │                 │    │   include)      │    │                 │          │
     │    └────────┬────────┘    └────────┬────────┘    └────────┬────────┘          │
     │             │                      │                      │                   │
     └─────────────┼──────────────────────┼──────────────────────┼───────────────────┘
                   │                      │                      │
═══════════════════╪══════════════════════╪══════════════════════╪═══════════════════════
                   │                      │                      │
     ┌─────────────┼──────────────────────┼──────────────────────┼───────────────────┐
     │ BACKEND (Go)│                      │                      │                   │
     │             ▼                      ▼                      ▼                   │
     │   ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐           │
     │   │  /auth/login    │    │   /auth/me      │    │  /auth/logout   │           │
     │   │                 │    │                 │    │                 │           │
     │   │ 1. Generate     │    │ 1. Get session  │    │ 1. Get session  │           │
     │   │    state (32B)  │    │    cookie       │    │    cookie       │           │
     │   │ 2. Set state in │    │ 2. Unseal/      │    │ 2. Revoke at    │           │
     │   │    cookie (15m) │    │    decrypt      │    │    WorkOS       │           │
     │   │ 3. Redirect to  │    │ 3. Check expiry │    │ 3. Clear cookie │           │
     │   │    WorkOS       │    │ 4. Auto-refresh │    │ 4. Redirect to  │           │
     │   │                 │    │    if expired   │    │    WorkOS logout│           │
     │   └────────┬────────┘    └─────────────────┘    └─────────────────┘           │
     │            │                                                                  │
     └────────────┼──────────────────────────────────────────────────────────────────┘
                  │
══════════════════╪══════════════════════════════════════════════════════════════════════
                  │
     ┌────────────┼───────────────────────────────────────────────────────────────────┐
     │ WORKOS     │                                                                   │
     │            ▼                                                                   │
     │  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐           │
     │  │  AuthKit SSO    │     │  Token Exchange │     │  Token Refresh  │           │
     │  │  Login Page     │────▶│  via /callback  │     │  API            │           │
     │  │                 │     │                 │     │                 │           │
     │  │  - Google       │     │  Returns:       │     │  Returns:       │           │
     │  │  - Microsoft    │     │  - Access Token │     │  - New Access   │           │
     │  │  - GitHub       │     │  - Refresh Token│     │  - New Refresh  │           │
     │  │  - Email/Pass   │     │  - User Info    │     │                 │           │
     │  └─────────────────┘     └────────┬────────┘     └─────────────────┘           │
     │                                   │                                            │
     └───────────────────────────────────┼────────────────────────────────────────────┘
                                         │
═════════════════════════════════════════╪══════════════════════════════════════════════
                                         │
     ┌───────────────────────────────────┼────────────────────────────────────────────┐
     │ CALLBACK FLOW                     ▼                                            │
     │                         ┌─────────────────┐                                    │
     │                         │ /auth/callback  │                                    │
     │                         └────────┬────────┘                                    │
     │                                  │                                             │
     │     ┌────────────────────────────┼────────────────────────────────┐            │
     │     │                            │                                │            │
     │     ▼                            ▼                                ▼            │
     │ ┌────────┐              ┌─────────────────┐              ┌─────────────────┐   │
     │ │Validate│              │ Exchange code   │              │ Create Session  │   │
     │ │state   │─────────────▶│ for tokens      │─────────────▶│                 │   │
     │ │param   │              │ (WorkOS API)    │              │ 1. Encrypt w/   │   │
     │ └────────┘              └─────────────────┘              │    AES-256-GCM  │   │
     │                                                          │ 2. Store in     │   │
     │                                                          │    HttpOnly     │   │
     │                                                          │    cookie       │   │
     │                                                          └────────┬────────┘   │
     │                                                                   │            │
     │                                                                   ▼            │
     │                                                          ┌─────────────────┐   │
     │                                                          │ Redirect to     │   │
     │                                                          │ Frontend        │   │
     │                                                          └─────────────────┘   │
     └────────────────────────────────────────────────────────────────────────────────┘
```

## Session Cookie Structure

The session is stored in an encrypted, HttpOnly cookie named `astro_session`.

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                              SESSION COOKIE STRUCTURE                                   │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                         │
│    Cookie: "astro_session"                                                              │
│    Attributes: HttpOnly, SameSite=Lax, Secure (production)                              │
│                                                                                         │
│    ┌─────────────────────────────────────────────────────────────────────────────┐      │
│    │  Encrypted with AES-256-GCM                                                 │      │
│    │  Key derived via PBKDF2-HMAC-SHA256 (600,000 iterations)                    │      │
│    │  ┌───────────────────────────────────────────────────────────────────────┐  │      │
│    │  │  SessionData {                                                        │  │      │
│    │  │    Session: {                                                         │  │      │
│    │  │      ID, UserID, OrganizationID,                                      │  │      │
│    │  │      WorkOSMembershipID, Role, Permissions, ...                       │  │      │
│    │  │      AccessToken:    string   // JWT for API calls                    │  │      │
│    │  │      RefreshToken:   string   // Token for session refresh            │  │      │
│    │  │      ExpiresAt, CreatedAt: time                                       │  │      │
│    │  │    }                                                                  │  │      │
│    │  │    User: {                                                            │  │      │
│    │  │      ID, Email, FirstName, LastName, EmailVerified, ...               │  │      │
│    │  │    }                                                                  │  │      │
│    │  │  }                                                                    │  │      │
│    │  └───────────────────────────────────────────────────────────────────────┘  │      │
│    └─────────────────────────────────────────────────────────────────────────────┘      │
│                                                                                         │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

## API Authentication (Bearer Token)

For API clients, authentication can also be done via Bearer token in the Authorization header.

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                              API AUTHENTICATION FLOW                                    │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                         │
│    Request ──▶ RequireAuth() Middleware                                                 │
│                     │                                                                   │
│                     ├──▶ Check "Authorization: Bearer <token>" header                   │
│                     │         │                                                         │
│                     │         ├──▶ Validate JWT signature via JWKS                      │
│                     │         ├──▶ Validate issuer (iss claim)                          │
│                     │         ├──▶ Validate audience (aud claim)                        │
│                     │         ├──▶ Check expiration                                     │
│                     │         ├──▶ Machine token (aud contains its own sub)?            │
│                     │         │         └──▶ Resolve app by client ID, set App context, │
│                     │         │             no User (scopes fill Session.Permissions)   │
│                     │         └──▶ Otherwise: set User + Session in request context     │
│                     │                                                                   │
│                     └──▶ Check "astro_session" cookie (fallback)                        │
│                               │                                                         │
│                               ├──▶ Decrypt session data (AES-256-GCM)                   │
│                               ├──▶ Validate session expiry                              │
│                               └──▶ Set user/session in request context                  │
│                                                                                         │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

A machine (M2M) token is discriminated from a user token by `isMachineToken`
(`internal/middleware/auth.go`): a machine token names its own client in
both `aud` and `sub`, while a WorkOS user access token carries no `aud`
claim at all. A machine token resolves to an `App` on the request context,
never a `User`; its scopes fill `Session.Permissions` so the rest of the
codebase's permission checks read one field for both caller kinds.

## Security Measures

### Cookie Security

| Attribute | Value | Purpose |
|-----------|-------|---------|
| `HttpOnly` | `true` | Prevents JavaScript access (XSS protection) |
| `SameSite` | `Lax` | Prevents CSRF attacks on POST/PUT/DELETE |
| `Secure` | Configurable | Ensures HTTPS-only transmission |
| `Domain` | Configurable | Restricts cookie scope |

### Encryption

| Component | Algorithm | Details |
|-----------|-----------|---------|
| Session encryption | AES-256-GCM | Authenticated encryption with random nonces |
| Key derivation | PBKDF2-HMAC-SHA256 | 600,000 iterations (OWASP 2023 recommendation) |
| State parameter | crypto/rand | 32 bytes of cryptographic randomness |

### Token Validation

| Check | Implementation |
|-------|----------------|
| Signature | RSA verification via JWKS (cached 1 hour) |
| Issuer (`iss`) | Must match `https://api.workos.com`, or its `/user_management/<client_id>` form |
| Audience (`aud`) | Not checked. WorkOS access tokens carry no `aud` claim, so validation passes an empty audience by design (`internal/auth/jwt.go`). |
| Expiration (`exp`) | Must not be expired |

### CSRF Protection

1. **OAuth State Parameter**: Random 32-byte state stored in short-lived cookie, validated on callback
2. **SameSite Cookies**: `SameSite=Lax` blocks cross-site POST requests

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `WORKOS_API_KEY` | Yes | - | WorkOS API key |
| `WORKOS_CLIENT_ID` | Yes | - | WorkOS client ID |
| `AUTH_JWT_ISSUER` | No | `https://api.workos.com` | Expected `iss` claim for Bearer-token validation |
| `WORKOS_REDIRECT_URI` | Yes | `http://localhost:8080/auth/callback` | OAuth callback URL |
| `FRONTEND_URL` | No | `http://localhost:5173` | Frontend redirect URL |
| `AUTH_COOKIE_NAME` | No | `astro_session` | Session cookie name |
| `AUTH_COOKIE_PASSWORD` | Yes | - | Encryption key (min 32 chars) |
| `AUTH_COOKIE_DOMAIN` | No | - | Cookie domain |
| `AUTH_COOKIE_SECURE` | No | `false` | Require HTTPS |
| `AUTH_COOKIE_SAMESITE` | No | `Lax` | SameSite attribute |
| `AUTH_COOKIE_MAX_AGE` | No | `720h` (30 days) | Cookie lifetime |
| `AUTH_SESSION_MAX_AGE` | No | `720h` (30 days) | Session lifetime |

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/auth/login` | GET | Initiates OAuth flow, redirects to WorkOS |
| `/auth/callback` | GET | Handles OAuth callback from WorkOS |
| `/auth/logout` | GET | Clears session, redirects to WorkOS logout |
| `/auth/me` | GET | Returns current user info |
| `/auth/refresh` | POST | Explicitly refreshes the session |
| `/auth/switch-org` | POST | Re-scopes the current session to a different WorkOS org the user belongs to |

## Active Account and Session Scope

The session JWT carries one WorkOS organization, so the dashboard works in one
account at a time. Two cookies track that:

| Cookie | Written by | Read by |
|--------|------------|---------|
| `astro_session` (org claim) | `/auth/callback`, `/auth/refresh`, `/auth/switch-org` | `RequireAccountPermission`, which refuses any account the claim does not cover |
| `astro:active-account` | the dashboard's account switcher | SSR loaders, to pick the account a page reads |

The two disagree routinely. Four client callers re-scope the session (the
account switcher, the deploy vault, blueprint creation, and org settings), and
only the switcher moves `astro:active-account`. That cookie also outlives the
session it was written for, and a fresh login scopes to the personal
organization.

`resolveActiveAccount` in `apps/astro-client/src/lib/active-account.ts` settles
the disagreement wherever the scope is derived: `getActiveAccount` and the root
loader drop an active-account cookie naming an account outside the session's
organization. The session claim wins, because the server refuses reads on any
other account.

## Client-Side Integration

The React frontend uses the `AuthProvider` component (`apps/astro-client/src/lib/AuthProvider.tsx`) to manage authentication state:

```typescript
// Check authentication on mount, unless already hydrated from the server-rendered response.
useEffect(() => {
  if (!hydratedRef.current) checkAuth();
}, [checkAuth]);

// Re-validate session when the tab regains visibility or the window gains
// focus. checkAuth() dedupes concurrent calls: with WorkOS refresh-token
// rotation, two simultaneous /me requests would race on the same refresh
// token and log the user out.
useEffect(() => {
  if (!state.isAuthenticated) return;
  const handleVisibilityChange = () => {
    if (document.visibilityState === 'visible') checkAuth();
  };
  const handleFocus = () => checkAuth();
  document.addEventListener('visibilitychange', handleVisibilityChange);
  window.addEventListener('focus', handleFocus);
  return () => {
    document.removeEventListener('visibilitychange', handleVisibilityChange);
    window.removeEventListener('focus', handleFocus);
  };
}, [state.isAuthenticated, checkAuth]);
```
