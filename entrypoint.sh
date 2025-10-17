#!/bin/sh
set -e

# Default to UID/GID 1000 if not specified
PUID=${PUID:-1000}
PGID=${PGID:-1000}

echo "Starting TapeDeck with UID:GID ${PUID}:${PGID}"

# Determine which group to use for the tapedeck user
if ! getent group tapedeck > /dev/null 2>&1; then
    # Check if PGID is already taken by another group
    if getent group "${PGID}" > /dev/null 2>&1; then
        # GID is taken - use the existing group instead
        EXISTING_GROUP=$(getent group "${PGID}" | cut -d: -f1)
        echo "GID ${PGID} already in use by group '${EXISTING_GROUP}', using it for tapedeck user"
        GROUP_NAME="${EXISTING_GROUP}"
    else
        # GID is available - create tapedeck group
        addgroup -g "${PGID}" tapedeck
        GROUP_NAME="tapedeck"
    fi
else
    GROUP_NAME="tapedeck"
fi

# Create user if it doesn't exist
if ! getent passwd tapedeck > /dev/null 2>&1; then
    adduser -D -u "${PUID}" -G "${GROUP_NAME}" tapedeck
fi

# Ensure directories have correct ownership
chown -R tapedeck:${GROUP_NAME} /app /data

# Execute the application as the tapedeck user
exec su-exec tapedeck "$@"
