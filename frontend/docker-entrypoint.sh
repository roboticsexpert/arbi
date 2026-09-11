#!/bin/sh
set -e

# Publish the backend URL to the browser at container start, so the built image
# is not pinned to a single API host.
cat > /srv/config.js <<EOF
window.__ARBI_CONFIG__ = { apiUrl: "${API_URL:-}" }
EOF

exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
