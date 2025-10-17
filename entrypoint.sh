#!/bin/sh
set -e

# Default to UID/GID 1000 if not specified
PUID=${PUID:-1000}
PGID=${PGID:-1000}

echo "Starting TapeDeck with UID:GID ${PUID}:${PGID}"

# Create group if it doesn't exist
if ! getent group tapedeck > /dev/null 2>&1; then
    addgroup -g "${PGID}" tapedeck
fi

# Create user if it doesn't exist
if ! getent passwd tapedeck > /dev/null 2>&1; then
    adduser -D -u "${PUID}" -G tapedeck tapedeck
fi

# Ensure directories have correct ownership
chown -R tapedeck:tapedeck /app /data

# Execute the application as the tapedeck user
exec su-exec tapedeck "$@"
