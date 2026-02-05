#!/bin/sh
set -e

# API_URL defaults to empty string (uses relative URLs with nginx proxy)
API_URL="${API_URL:-}"

# Generate env-config.js from template with environment variable substitution
envsubst < /usr/share/nginx/html/env-config.template.js > /usr/share/nginx/html/env-config.js

# Remove template file
rm -f /usr/share/nginx/html/env-config.template.js

echo "Generated env-config.js with API_URL=${API_URL}"

# Execute the main command (nginx)
exec "$@"
