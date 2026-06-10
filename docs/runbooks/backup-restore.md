# Backup & Restore Runbook

**Purpose**: Backup and restore the Conscience database.
**Severity**: Critical (data safety)
**Estimated Time**: Backup: 5 min, Restore: 10-30 min (depends on size)

---

## Prerequisites

- `pg_dump` / `pg_restore` (PostgreSQL 16+, matching server version)
- Sufficient disk space for backup dump
- Database connection credentials

---

## Postgres Backup

### Standard Backup (pg_dump)

```bash
# Full database backup (custom format — recommended)
pg_dump -h <host> -p 5432 -U <user> \
  --format=custom \
  --compress=9 \
  --no-owner \
  --dbname=conscience \
  -f conscience_$(date +%Y%m%d_%H%M%S).dump

# Plain SQL backup (portable, larger)
pg_dump -h <host> -p 5432 -U <user> \
  --format=plain \
  --no-owner \
  --dbname=conscience \
  -f conscience_$(date +%Y%m%d_%H%M%S).sql
```

### Schema-Only Backup (migration state)

```bash
pg_dump -h <host> -p 5432 -U <user> \
  --schema-only \
  --dbname=conscience \
  -f conscience_schema_$(date +%Y%m%d).sql
```

### Selective Backup (key tables)

```bash
# Backup only critical operational tables
pg_dump -h <host> -p 5432 -U <user> \
  --table=sessions \
  --table=memory_events \
  --table=api_keys \
  --table=tasks \
  --table=audit_logs \
  --format=custom \
  --dbname=conscience \
  -f conscience_core_$(date +%Y%m%d).dump
```

---

## Postgres Restore

### Restore from Custom Format

```bash
# Drop and recreate the database (requires admin privileges)
dropdb -h <host> -p 5432 -U <user> conscience
createdb -h <host> -p 5432 -U <user> conscience

# Restore from custom dump
pg_restore -h <host> -p 5432 -U <user> \
  --dbname=conscience \
  --no-owner \
  --verbose \
  conscience_20260529_120000.dump
```

### Restore from SQL Dump

```bash
psql -h <host> -p 5432 -U <user> -d conscience \
  -f conscience_20260529_120000.sql
```

### Point-in-Time Recovery (if WAL archiving configured)

```bash
# 1. Restore base backup
pg_restore -h <host> -d conscience conscience_base.dump

# 2. Apply WAL up to target time
# (Requires WAL archiving to be configured — see PostgreSQL docs)
```

---

## SQLite Backup

### Online Backup (while server is running)

```bash
# Use sqlite3 .backup command
sqlite3 dev.db ".backup 'backup_$(date +%Y%m%d).db'"

# Or use VACUUM INTO (SQLite 3.27+)
sqlite3 dev.db "VACUUM INTO 'backup_$(date +%Y%m%d).db'"
```

### Offline Backup (server stopped)

```bash
cp dev.db dev.db.backup_$(date +%Y%m%d)
```

### Restore SQLite

```bash
cp backup_20260529.db dev.db
# Restart the server — it reads the replaced database file
```

---

## Backup Verification

```bash
# Verify the dump file is valid
pg_restore --list conscience_20260529.dump | head -20

# Test restore to a separate database
createdb conscience_restore_test
pg_restore -d conscience_restore_test conscience_20260529.dump
psql -d conscience_restore_test -c "SELECT count(*) FROM sessions;"
dropdb conscience_restore_test
```

---

## Retention Policy

| Backup Type | Retention | Frequency |
|-------------|-----------|-----------|
| Daily full backup | 30 days | Daily (off-peak) |
| Weekly full backup | 12 weeks | Weekly |
| Monthly snapshot | 12 months | Monthly |
| Schema backup | Permanent | On each migration change |

---

## Troubleshooting

### Backup fails with "permission denied"

```bash
# Grant required privileges
psql -h <host> -U postgres -d conscience \
  -c "GRANT CONNECT ON DATABASE conscience TO <backup_user>;"
psql -h <host> -U postgres -d conscience \
  -c "GRANT USAGE ON SCHEMA public TO <backup_user>;"
psql -h <host> -U postgres -d conscience \
  -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO <backup_user>;"
```

### Restore fails "role does not exist"

Add `--no-owner` flag to pg_restore (included in commands above).

### Disk space low during restore

```bash
# Check available space
df -h /var/lib/postgresql/

# Use compressed dump format (already compressed above)
# Or stream the restore directly
pg_dump ... | gzip > dump.sql.gz
gunzip -c dump.sql.gz | psql -d conscience
```

---

> **Trace**: `axiom:trace work_item=WI-019 spec=specs/009-deployment.md doc=docs/runbooks/backup-restore.md`
