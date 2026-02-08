#!/bin/sh
set -e

# BACKEND_URL: where nginx proxies /api and /health requests
# Defaults to astro-messaging sidecar (used in ast dev)
BACKEND_URL="${BACKEND_URL:-http://astro-messaging:8080}"

# API_URL: frontend override (empty = use relative URLs through nginx proxy)
API_URL="${API_URL:-}"

# Generate nginx config from template
envsubst '${BACKEND_URL}' < /etc/nginx/nginx.conf.template > /etc/nginx/conf.d/default.conf

# When using Docker compose (default backend), add resolver + variable pattern
# so nginx can handle runtime DNS resolution (e.g. if astro-messaging restarts).
# For external URLs we use direct proxy_pass which resolves via /etc/hosts.
if [ "$BACKEND_URL" = "http://astro-messaging:8080" ]; then
    sed -i 's|proxy_pass http://astro-messaging:8080;|resolver 127.0.0.11 valid=10s ipv6=off;\n        set $backend http://astro-messaging:8080;\n        proxy_pass $backend;|g' \
        /etc/nginx/conf.d/default.conf
fi

# Generate env-config.js from template
envsubst < /usr/share/nginx/html/env-config.template.js > /usr/share/nginx/html/env-config.js

# Remove template file
rm -f /usr/share/nginx/html/env-config.template.js

echo "Generated nginx config with BACKEND_URL=${BACKEND_URL}"
echo "Generated env-config.js with API_URL=${API_URL}"

# Execute the main command (nginx)
exec "$@"
