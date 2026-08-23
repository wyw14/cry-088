#!/usr/bin/env sh
set -eu
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/002_seed.sql
