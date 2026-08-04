# Queen mTLS Certificate Setup

How to set up mTLS certificates for connecting `astro-queen` to the admin gRPC servers.

## Prerequisites

- 1Password desktop app running (or `op` CLI installed)
- Access to the **Astro** vault in 1Password (item: `queen-bee-client`)

## Environments

| Environment | Server address           | Queen command         |
| ----------- | ------------------------ | --------------------- |
| Production  | `admin.astropods.ai:443` | `queen prod admin`    |
| Preview     | `admin.astropod.ai:443`  | `queen preview admin` |

## Setup via 1Password

Queen fetches client certificates directly from 1Password. Run:

```bash
queen login
```

This prompts for your 1Password account name (e.g. `you@example.com`), then:

1. Authenticates via the 1Password desktop app
2. Resolves certs from the `Astro` vault:
   - `op://Astro/queen-bee-client/client-cert`
   - `op://Astro/queen-bee-client/client-key`
   - `op://Astro/queen-bee-client/ca-cert`
3. Writes PEM files to `~/.astro-queen/` with `0600` permissions:
   - `client.crt`, `client.key`, `ca.crt`
4. Writes `~/.astro-queen/config.yaml`:
   ```yaml
   op_account: "you@example.com"
   ```

After login, connect to either environment:

```bash
queen prod admin     # admin.astropods.ai:443
queen preview admin  # admin.astropod.ai:443
```

Both environments use the same client certs from the shared 1Password item.

## How it works at runtime

1. Queen loads `~/.astro-queen/config.yaml` (only stores `op_account`)
2. Cert files are read from `~/.astro-queen/{client.crt, client.key, ca.crt}`
3. mTLS connection is established with TLS 1.3 minimum
4. If cert files are missing, queen falls back to insecure gRPC (local dev only)

## Manual cert generation

If you need certs outside 1Password (e.g. using your own CA or for automation). The CA is generated once and reused to sign all server and client certs.

### Step 1: Create the CA (once)
**Already generate and stored in 1Password, so skip this step and reuse**

```bash
mkdir -p .certs && cd .certs
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt -subj "/CN=astro-admin-ca"
```

Keep `ca.key` secure — it signs everything below. The CA is valid for 10 years. The `.certs/` directory is gitignored.

### Step 2: Generate server certs

Generate a cert for each environment the server runs in.

Server certs must include a SAN (Subject Alternative Name) — Go's TLS library rejects certs that only use the legacy CN field.

**Production (`admin.astropods.ai`)**

```bash
cd .certs
openssl genrsa -out prod.key 2048
openssl req -new -key prod.key -out prod.csr -subj "/CN=admin.astropods.ai" \
  -addext "subjectAltName=DNS:admin.astropods.ai"
openssl x509 -req -days 365 -in prod.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -copy_extensions copyall -out prod.crt
```

**Preview (`admin.astropod.ai`)**

```bash
cd .certs
openssl genrsa -out preview.key 2048
openssl req -new -key preview.key -out preview.csr -subj "/CN=admin.astropod.ai" \
  -addext "subjectAltName=DNS:admin.astropod.ai"
openssl x509 -req -days 365 -in preview.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -copy_extensions copyall -out preview.crt
```

**Local (`localhost`)**

```bash
cd .certs
openssl genrsa -out local-server.key 2048
openssl req -new -key local-server.key -out local-server.csr -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
openssl x509 -req -days 365 -in local-server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -copy_extensions copyall -out local-server.crt
```

### Step 3: Generate client cert (once)

One client cert works for all environments since they share the same CA.

```bash
cd .certs
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr -subj "/CN=astro-queen"
openssl x509 -req -days 365 -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt
```

### Step 4: Install client certs for queen

```bash
cp .certs/client.crt ~/.astro-queen/client.crt
cp .certs/client.key ~/.astro-queen/client.key
cp .certs/ca.crt     ~/.astro-queen/ca.crt
chmod 0600 ~/.astro-queen/client.key
```

### Step 5: Configure astro-server

Add the server cert for the relevant environment to `apps/astro-server/.env`:

```bash
ADMIN_GRPC_PORT=9091
ADMIN_GRPC_CERT_FILE=.certs/prod.crt    # or preview.crt / local-server.crt
ADMIN_GRPC_KEY_FILE=.certs/prod.key     # or preview.key / local-server.key
ADMIN_GRPC_CA_FILE=.certs/ca.crt
```
