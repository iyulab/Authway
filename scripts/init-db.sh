#!/bin/bash
set -e

# Create hydra database if it doesn't exist
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE hydra'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'hydra')\gexec
EOSQL

echo "Hydra database created successfully"
