# Astro CLI Authentication

This document describes how authentication works in the Astro CLI.

## Overview

The CLI uses OAuth 2.0 Device Authorization Flow (RFC 8628) via WorkOS. This allows users to authenticate via a browser while keeping the CLI stateless.

## Authentication Flow

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                         CLI DEVICE AUTHORIZATION FLOW                         │
└──────────────────────────────────────────────────────────────────────────────┘

  ┌─────────┐                      ┌─────────┐                    ┌──────────┐
  │   CLI   │                      │ WorkOS  │                    │ Browser  │
  └────┬────┘                      └────┬────┘                    └────┬─────┘
       │                                │                              │
       │  1. POST /authorize/device     │                              │
       │  ─────────────────────────────>│                              │
       │                                │                              │
       │  device_code, user_code, URL   │                              │
       │  <─────────────────────────────│                              │
       │                                │                              │
       │  2. Display code, open browser │                              │
       │  ─────────────────────────────────────────────────────────────>
       │                                │                              │
       │                                │   3. User enters code        │
       │                                │   <──────────────────────────│
       │                                │                              │
       │  4. Poll /authenticate         │                              │
       │  ─────────────────────────────>│                              │
       │     (repeat until success)     │                              │
       │                                │                              │
       │  5. access_token, refresh_token│                              │
       │  <─────────────────────────────│                              │
       │                                │                              │
       │  6. Store tokens               │                              │
       ▼                                ▼                              ▼
```

### Steps

1. User runs `ast login`
2. CLI requests device authorization from WorkOS
3. CLI displays user code (e.g., `ABCD-EFGH`) and opens verification URL
4. User enters code in browser and authenticates (Google, GitHub, etc.)
5. CLI polls WorkOS until authentication completes
6. Tokens are stored securely (keyring or fallback file)

## Credential Storage

### Primary: System Keyring

When available, tokens are stored in the OS keyring:
- **macOS**: Keychain
- **Linux**: Secret Service (GNOME Keyring, KWallet)
- **Windows**: Credential Manager

Keys use format: `{profile}_{token_type}` (e.g., `default_access_token`)

### Fallback: File Storage

When keyring is unavailable, credentials are stored in `~/.ast/credentials.json` with permissions `0600`.

```
~/.ast/
└── credentials.json    # Profile metadata + accounts (tokens stored here only when the keyring is unavailable)
```

The server URL is not stored per profile; it is fixed at build time (`buildinfo.DefaultServerURL`). The registry URL is derived from it at runtime as `registry.<hostname>` (via `RegistryURLFromServerURL`), unless a build overrides it with `buildinfo.DefaultRegistryURL`.

### Profile Structure

```json
{
  "profiles": {
    "default": {
      "expires_at": "2026-01-15T10:00:00Z",
      "user": {
        "id": "user_123",
        "email": "user@example.com",
        "first_name": "Jane",
        "last_name": "Doe"
      },
      "accounts": [
        { "id": "acct_1", "name": "jane", "type": "personal" },
        { "id": "org_1", "name": "my-org", "type": "organization", "role": "admin", "workos_org_id": "org_..." }
      ],
      "current_account": "my-org",
      "previous_account": "jane"
    }
  },
  "current_profile": "default"
}
```

(`access_token`/`refresh_token` fields also appear here when the keyring is unavailable; otherwise they live in the keyring.)

### Active account on re-login

The active account (`current_account`) is stored in the profile alongside tokens. `ast account switch` updates it; `ast login` preserves it when you re-authenticate, as long as you still belong to that account. If membership was removed, the CLI falls back to your personal account and prints a note. Use `ast login --account <name>` to override the restored selection.

## Token Lifecycle

| Token         | Lifetime   | Purpose                           |
| ------------- | ---------- | --------------------------------- |
| Access Token  | ~1 hour    | API authentication (Bearer token) |
| Refresh Token | Long-lived | Obtain new access tokens          |

### Automatic Refresh

Tokens are refreshed automatically when within 5 minutes of expiry. The CLI checks token validity on each authenticated request and refreshes if needed.

## CLI Commands

| Command                       | Description                                      |
| ----------------------------- | ------------------------------------------------ |
| `ast login`                   | Authenticate via device flow; restores last active account |
| `ast login --no-browser`      | Print URL instead of opening browser             |
| `ast login --account <name>`  | Switch to this account after login               |
| `ast logout`                  | Clear stored credentials                         |
| `ast whoami`                  | Display current user info                        |

## Security Measures

1. **Secure Storage**: Tokens stored in OS keyring when available
2. **File Permissions**: `~/.ast` directory `0700`, files `0600`
3. **Token Isolation**: Environment tokens cleared after first read
4. **Automatic Expiry**: Tokens validated before use, refreshed proactively
