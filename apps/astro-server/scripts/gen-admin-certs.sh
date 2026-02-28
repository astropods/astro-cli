#!/usr/bin/env bash
# Generates self-signed mTLS certs for local admin gRPC development.
# Outputs to apps/astro-server/.certs/ and writes ~/.astro-queen/config.yaml.
set -euo pipefail

cd "$(dirname "$0")/.."

CERTS_DIR="$(pwd)/.certs"
QUEEN_DIR="$HOME/.astro-queen"

mkdir -p "$CERTS_DIR" "$QUEEN_DIR"

echo "==> Generating CA..."
openssl genrsa -out "$CERTS_DIR/ca.key" 4096 2>/dev/null
openssl req -new -x509 -days 3650 -key "$CERTS_DIR/ca.key" \
  -out "$CERTS_DIR/ca.crt" -subj "/CN=astro-admin-ca" 2>/dev/null

echo "==> Generating server cert..."
openssl genrsa -out "$CERTS_DIR/server.key" 2048 2>/dev/null
openssl req -new -key "$CERTS_DIR/server.key" \
  -out "$CERTS_DIR/server.csr" -subj "/CN=localhost" 2>/dev/null
openssl x509 -req -days 365 \
  -in "$CERTS_DIR/server.csr" \
  -CA "$CERTS_DIR/ca.crt" -CAkey "$CERTS_DIR/ca.key" -CAcreateserial \
  -out "$CERTS_DIR/server.crt" 2>/dev/null

echo "==> Generating client cert..."
openssl genrsa -out "$CERTS_DIR/client.key" 2048 2>/dev/null
openssl req -new -key "$CERTS_DIR/client.key" \
  -out "$CERTS_DIR/client.csr" -subj "/CN=astro-queen" 2>/dev/null
openssl x509 -req -days 365 \
  -in "$CERTS_DIR/client.csr" \
  -CA "$CERTS_DIR/ca.crt" -CAkey "$CERTS_DIR/ca.key" -CAcreateserial \
  -out "$CERTS_DIR/client.crt" 2>/dev/null

echo "==> Copying client certs to $QUEEN_DIR..."
cp "$CERTS_DIR/ca.crt"     "$QUEEN_DIR/ca.crt"
cp "$CERTS_DIR/client.crt" "$QUEEN_DIR/client.crt"
cp "$CERTS_DIR/client.key" "$QUEEN_DIR/client.key"

echo "==> Writing $QUEEN_DIR/config.yaml..."
cat > "$QUEEN_DIR/config.yaml" <<EOF
server: "localhost:9091"
cert_file: "$QUEEN_DIR/client.crt"
key_file:  "$QUEEN_DIR/client.key"
ca_file:   "$QUEEN_DIR/ca.crt"
EOF

echo ""
echo "Done. Add these to your astro-server .env:"
echo ""
echo "  ADMIN_GRPC_PORT=9091"
echo "  ADMIN_GRPC_CERT_FILE=$CERTS_DIR/server.crt"
echo "  ADMIN_GRPC_KEY_FILE=$CERTS_DIR/server.key"
echo "  ADMIN_GRPC_CA_FILE=$CERTS_DIR/ca.crt"
echo ""
echo "Then run:  moon run astro-queen:run-tls"
