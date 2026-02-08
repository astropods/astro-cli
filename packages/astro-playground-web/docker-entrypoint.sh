#!/bin/sh
set -e

# BACKEND_URL: where nginx proxies /api and /health requests
# Defaults to astro-messaging sidecar (used in ast dev)
BACKEND_URL="${BACKEND_URL:-http://astro-messaging:8080}"

# API_URL: frontend override (empty = use relative URLs through nginx proxy)
API_URL="${API_URL:-}"

# Build the proxy_pass block. For Docker compose (default), use resolver +
# variable so nginx handles runtime DNS. For external URLs, use direct proxy_pass
# which resolves via /etc/hosts at startup.
if [ "$BACKEND_URL" = "http://astro-messaging:8080" ]; then
    PROXY_BLOCK='resolver 127.0.0.11 valid=10s ipv6=off;
        set $backend http://astro-messaging:8080;
        proxy_pass $backend'
else
    PROXY_BLOCK="proxy_pass ${BACKEND_URL}"
fi

# Generate nginx config — single-quoted NGINX heredoc prevents shell expansion,
# so we write it in two parts: static prefix, dynamic proxy block, static suffix.
cat > /etc/nginx/conf.d/default.conf <<'NGINX_HEAD'
server {
    listen 80;
    server_name localhost;
    root /usr/share/nginx/html;
    index index.html;

    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_proxied expired no-cache no-store private auth;
    gzip_types text/plain text/css text/xml text/javascript application/x-javascript application/xml application/javascript;

    location /api/ {
NGINX_HEAD

cat >> /etc/nginx/conf.d/default.conf <<NGINX_PROXY
        ${PROXY_BLOCK};
NGINX_PROXY

cat >> /etc/nginx/conf.d/default.conf <<'NGINX_MID'
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE support
        proxy_set_header Connection '';
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding off;
    }

    location /health {
NGINX_MID

cat >> /etc/nginx/conf.d/default.conf <<NGINX_PROXY2
        ${PROXY_BLOCK};
NGINX_PROXY2

cat >> /etc/nginx/conf.d/default.conf <<'NGINX_TAIL'
        proxy_http_version 1.1;
        proxy_set_header Host $host;
    }

    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires -1;
        add_header Cache-Control "no-store, no-cache, must-revalidate";
    }

    location = /env-config.js {
        expires -1;
        add_header Cache-Control "no-store, no-cache, must-revalidate";
    }

    location / {
        try_files $uri $uri/ /index.html;
    }

    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
}
NGINX_TAIL

# Generate env-config.js from template
envsubst < /usr/share/nginx/html/env-config.template.js > /usr/share/nginx/html/env-config.js
rm -f /usr/share/nginx/html/env-config.template.js

echo "Generated nginx config with BACKEND_URL=${BACKEND_URL}"
echo "Generated env-config.js with API_URL=${API_URL}"

exec "$@"
