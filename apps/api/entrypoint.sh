#!/bin/sh
set -e

echo "Running migrations"
migration_attempt=1
max_migration_attempts=12
until goose -dir /app/db/migrations postgres "$DATABASE_URL" up; do
  if [ "$migration_attempt" -ge "$max_migration_attempts" ]; then
    echo "Migrations failed after $migration_attempt attempts"
    exit 1
  fi

  echo "Migrations failed on attempt $migration_attempt; retrying in 5 seconds"
  migration_attempt=$((migration_attempt + 1))
  sleep 5
done

if [ "$RUN_DB_SEED" = "true" ]; then
  echo "Running seed"
  /app/seed
else
  echo "Skipping seed; set RUN_DB_SEED=true to seed initial email data"
fi

if [ "$RUN_DEMO_SEED" = "true" ]; then
  if [ "$DEMO_RESET_ON_DEPLOY" = "true" ]; then
    echo "Running demo seed reset"
    /app/demoseed -reset
  else
    echo "Running demo seed"
    /app/demoseed
  fi
else
  echo "Skipping demo seed; set RUN_DEMO_SEED=true to seed demo data"
fi

echo "Starting server"
exec /app/server
