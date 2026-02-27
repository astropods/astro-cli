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

1. User runs `astro login`
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
~/.astro/
└── credentials.json    # Profile metadata and server/registry URLs (tokens in keyring if available)
```

Server and registry URLs are stored per profile. Registry is derived as `registry.<hostname>` from the host set at login (`ast login --host <url>`).

### Profile Structure

```json
{
  "profiles": {
    "default": {
      "server_url": "https://api.astro.example.com",
      "registry_url": "https://registry.astro.example.com",
      "expires_at": "2025-01-15T10:00:00Z",
      "user": {
        "id": "user_123",
        "email": "user@example.com",
        "first_name": "Jane",
        "last_name": "Doe"
      }
    }
  },
  "current_profile": "default"
}
```

## Token Lifecycle

| Token | Lifetime | Purpose |
|-------|----------|---------|
| Access Token | ~1 hour | API authentication (Bearer token) |
| Refresh Token | Long-lived | Obtain new access tokens |

### Automatic Refresh

Tokens are refreshed automatically when within 5 minutes of expiry. The CLI checks token validity on each authenticated request and refreshes if needed.

## CLI Commands

| Command | Description |
|---------|-------------|
| `astro login` | Authenticate via device flow |
| `astro login --no-browser` | Print URL instead of opening browser |
| `astro logout` | Clear stored credentials |
| `astro whoami` | Display current user info |

## Security Measures

1. **Secure Storage**: Tokens stored in OS keyring when available
2. **File Permissions**: `~/.astro` directory `0700`, files `0600`
3. **Token Isolation**: Environment tokens cleared after first read
4. **Automatic Expiry**: Tokens validated before use, refreshed proactively
