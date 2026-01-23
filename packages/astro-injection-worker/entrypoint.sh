#!/bin/sh
# Always fix permissions on state directory (volume mounts may override ownership)
echo "Ensuring /app/state is owned by worker user..."
chown -R worker:worker /app/state

# Run as worker user
exec su-exec worker /app/injection-worker "$@"
