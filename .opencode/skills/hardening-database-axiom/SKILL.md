---
name: hardening-database-axiom
description: >
  Database and data layer hardening for any codebase. Covers N+1 queries, connection
  pool exhaustion, transactions spanning HTTP calls, table-locking migrations, dual-write
  consistency, and resilience under DB failure. Migration findings always require
  requires_human_review: true. Produces HARDEN-DB-* findings with Tier-3+ verifiable
  acceptance criteria.
version: "1.0"
tags:
  vertical: [reliability, coding]
  category: hardening
  core: false
metadata:
  related_skills:
    - hardening-anti-patterns-axiom
    - hardening-sre-axiom
    - hardening-intake-axiom
    - db-architect-axiom
---

# Hardening: Database & Data Layer

> **"What happens if the primary DB is unreachable for 30 seconds? 5 minutes?"**
>
> **"The N+1 query is the most common performance bug that looks like a correctness bug."**

This skill audits database interactions for correctness, performance, and resilience. It is portable — no Axiom-internal dependencies.

**⚠️ All migration findings MUST have `requires_human_review: true`.** Migrations are irreversible operations that can cause data loss or production outages.

## When to Load This Skill

- Auditing a service with database interactions
- Before a major schema migration
- After a production DB performance incident
- When query latency is unexpectedly high
- When connection pool exhaustion is observed

---

## The Database Audit Prompt

Use this prompt (with the shared header from `hardening-anti-patterns-axiom`):

```
Review all database interactions in this codebase. Look for:

- Connection handling: pools that aren't bounded, connections that
  aren't released, missing timeouts, no retry on transient failures
- Query patterns: N+1 queries, missing indexes, full-table scans
- Transactions: missing transactions around multi-step writes,
  transactions held open across external calls
- Migrations: destructive migrations with no rollback, table-locking
  migrations during peak traffic
- Consistency: dual-writes to DB and cache with no invalidation
- Resilience: what happens if the primary DB is unreachable for
  30 seconds? 5 minutes?

For migrations specifically, flag as requires_human_review: true.
```

---

## Database Audit Checklist

### Connection Handling

- [ ] **Connection pool configured** — `pool_size`, `max_overflow`, `pool_timeout` set
- [ ] **Connection pool pre-ping** — `pool_pre_ping=True` to detect stale connections
- [ ] **Connection released on error** — `try/finally` or context manager used
- [ ] **Query timeout configured** — prevents runaway queries
- [ ] **Retry on transient failures** — connection refused, deadlock, serialization failure

### Query Patterns

- [ ] **No N+1 queries** — ForeignKey access uses `select_related()` or JOIN
- [ ] **No full-table scans** — WHERE clauses use indexed columns
- [ ] **Pagination on large result sets** — no `SELECT *` without LIMIT
- [ ] **Bulk operations for batch writes** — `bulk_create()` instead of loop of `save()`
- [ ] **Query logging enabled** — slow queries logged above threshold

### Transactions

- [ ] **Multi-step writes wrapped in transaction** — atomicity guaranteed
- [ ] **Transactions don't span HTTP calls** — no network I/O inside transaction
- [ ] **Transactions don't span user input** — no waiting for user inside transaction
- [ ] **Deadlock handling** — retry on deadlock detection
- [ ] **Transaction timeout** — long-running transactions killed

### Migrations

- [ ] **Expand/contract pattern** — no big-bang schema changes
- [ ] **No table-locking DDL** — `ALTER TABLE` uses `CONCURRENTLY` or equivalent
- [ ] **Rollback plan documented** — every migration has a down migration
- [ ] **Tested on production-size data** — migration timing verified
- [ ] **Deployed during low-traffic window** — or proven to be zero-downtime

### Consistency

- [ ] **Cache invalidation on write** — DB write + cache invalidation atomic or ordered
- [ ] **Dual-write uses transactional outbox** — DB + message queue consistency
- [ ] **Idempotent writes** — duplicate writes don't corrupt data

### Resilience

- [ ] **Graceful degradation on DB failure** — returns cached data or error, not crash
- [ ] **Read replica fallback** — reads route to replica when primary is slow
- [ ] **Connection pool exhaustion handled** — returns error, not hang

---

## Detection Patterns

### Grep Commands

```bash
# N+1 query risk: loop over queryset without prefetch
grep -rn "for.*in.*\.objects\.all()\|for.*in.*\.objects\.filter(" \
  --include="*.py" -A 5 | grep -v "select_related\|prefetch_related\|annotate"

# N+1 query risk: accessing related object in loop (Django)
grep -rn "\.author\.\|\.user\.\|\.category\." --include="*.py" \
  | grep -v "select_related\|prefetch_related"

# Unbounded connection pool
grep -rn "create_engine(" --include="*.py" \
  | grep -v "pool_size\|max_overflow\|pool_timeout"

# Transaction spanning HTTP call
grep -rn "with.*transaction\|@transaction\.atomic\|session\.begin()" \
  --include="*.py" -A 30 | grep "requests\.\|httpx\.\|urllib\."

# Missing transaction on multi-step write
grep -rn "\.save()\|\.commit()" --include="*.py" -B 10 \
  | grep -v "transaction\|atomic\|begin\|rollback"

# Table-locking DDL in migrations
grep -rn "ALTER TABLE\|ADD COLUMN\|DROP COLUMN\|RENAME COLUMN\|ADD INDEX" \
  --include="*.sql" --include="migrations/*.py" \
  | grep -v "CONCURRENTLY\|ALGORITHM=INPLACE\|LOCK=NONE"

# Dual-write without outbox
grep -rn "\.save()\|\.commit()" --include="*.py" -A 5 \
  | grep "queue\.\|publish\.\|send_message\|produce("
```

---

## Anti-Patterns with Fixes

### AP-DB-001: N+1 Query Problem

**Severity:** medium–high (depends on scale)

```python
# BAD: 1 query for posts, N queries for authors
# With 100 posts: 101 queries!
posts = Post.objects.all()
for post in posts:
    print(post.author.name)  # Separate DB query per post
```

**Fix (Django ORM):**
```python
# GOOD: select_related for ForeignKey (single JOIN query)
posts = Post.objects.select_related('author').all()
for post in posts:
    print(post.author.name)  # No additional query

# GOOD: prefetch_related for ManyToMany (2 queries, Python join)
posts = Post.objects.prefetch_related('tags').all()
for post in posts:
    for tag in post.tags.all():  # No additional query
        print(tag.name)
```

**Fix (SQLAlchemy):**
```python
# GOOD: joinedload for eager loading
from sqlalchemy.orm import joinedload

posts = session.query(Post).options(joinedload(Post.author)).all()
for post in posts:
    print(post.author.name)  # No additional query

# GOOD: subqueryload for collections
posts = session.query(Post).options(subqueryload(Post.tags)).all()
```

**Detection with Django Debug Toolbar:**
```python
# In development, enable query counting
from django.db import connection

def view(request):
    with connection.execute_wrapper(count_queries):
        result = get_posts()
    # Log query count; alert if > expected
```

---

### AP-DB-002: Unbounded Connection Pool

**Severity:** high

```python
# BAD: Default pool settings, no timeout
from sqlalchemy import create_engine

engine = create_engine("postgresql://user:pass@host/db")
# Default: pool_size=5, max_overflow=10, pool_timeout=30
# Under load: all 15 connections consumed, new requests hang
```

**Fix:**
```python
# GOOD: Explicit pool configuration
from sqlalchemy import create_engine
from sqlalchemy.pool import QueuePool

engine = create_engine(
    "postgresql://user:pass@host/db",
    poolclass=QueuePool,
    pool_size=10,           # Connections to keep open
    max_overflow=20,        # Additional connections under load
    pool_timeout=30,        # Seconds to wait for connection (raises TimeoutError)
    pool_recycle=3600,      # Recycle connections after 1 hour (prevents stale)
    pool_pre_ping=True,     # Test connection health before use
    connect_args={
        "connect_timeout": 10,      # Connection timeout
        "options": "-c statement_timeout=30000"  # Query timeout (30s)
    }
)
```

**PgBouncer for connection pooling at the infrastructure level:**
```ini
# pgbouncer.ini
[databases]
mydb = host=db-primary.internal port=5432 dbname=mydb

[pgbouncer]
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 20
reserve_pool_size = 5
reserve_pool_timeout = 5
```

---

### AP-DB-003: Transaction Spanning HTTP Call

**Severity:** high

```python
# BAD: Transaction held open during HTTP call
# DB locks held for entire HTTP duration (could be seconds)
def process_order(order_id):
    with db.transaction():
        order = Order.get(order_id)
        order.status = "processing"
        db.commit()
        
        # HTTP call INSIDE transaction — locks held!
        response = requests.post(
            "https://payment-service/charge",
            json={"amount": order.total},
            timeout=30  # 30 seconds of lock holding!
        )
        
        order.status = "completed" if response.ok else "failed"
        db.commit()
```

**Fix:**
```python
# GOOD: Short transactions, HTTP call outside
def process_order(order_id):
    # Transaction 1: Mark as processing (short)
    with db.transaction():
        order = Order.get(order_id)
        order.status = "processing"
        db.commit()
    
    # HTTP call outside any transaction
    try:
        response = requests.post(
            "https://payment-service/charge",
            json={"amount": order.total},
            timeout=30
        )
        success = response.ok
        payment_id = response.json().get("payment_id") if success else None
    except requests.RequestException as e:
        logger.error("payment_service_error", order_id=order_id, error=str(e))
        success = False
        payment_id = None
    
    # Transaction 2: Update based on result (short)
    with db.transaction():
        order = Order.get(order_id)
        order.status = "completed" if success else "failed"
        if payment_id:
            order.payment_id = payment_id
        db.commit()
```

---

### AP-DB-004: Table-Locking Migration

**Severity:** high | `requires_human_review: true`

```sql
-- BAD: Acquires ACCESS EXCLUSIVE lock, blocks all reads and writes
ALTER TABLE users ADD COLUMN phone VARCHAR(20);
-- On a table with 10M rows, this can take minutes!
```

**Fix — Expand/Contract Pattern:**

```sql
-- Step 1: Add nullable column (fast, minimal lock in PostgreSQL 11+)
ALTER TABLE users ADD COLUMN phone VARCHAR(20) DEFAULT NULL;
-- PostgreSQL 11+: No table rewrite for nullable columns with DEFAULT NULL

-- Step 2: Deploy code that writes to BOTH old and new columns
-- (Application handles dual-write during transition)

-- Step 3: Backfill existing rows in small batches (no lock)
DO $$
DECLARE
  batch_size INT := 1000;
  last_id BIGINT := 0;
  max_id BIGINT;
BEGIN
  SELECT MAX(id) INTO max_id FROM users;
  WHILE last_id < max_id LOOP
    UPDATE users 
    SET phone = old_phone_field
    WHERE id > last_id AND id <= last_id + batch_size
      AND phone IS NULL AND old_phone_field IS NOT NULL;
    last_id := last_id + batch_size;
    PERFORM pg_sleep(0.1);  -- Brief pause between batches
  END LOOP;
END $$;

-- Step 4: Deploy code that reads from new column only

-- Step 5: Drop old column (after verification, separate migration)
ALTER TABLE users DROP COLUMN old_phone_field;
```

**PostgreSQL DDL Lock Reference:**

| Operation | Lock Type | Blocks |
|---|---|---|
| `ALTER TABLE ADD COLUMN` (nullable, no default) | ACCESS EXCLUSIVE | Everything |
| `ALTER TABLE ADD COLUMN` (with DEFAULT, PG 11+) | ACCESS EXCLUSIVE | Everything (but fast) |
| `CREATE INDEX` | SHARE | Writes |
| `CREATE INDEX CONCURRENTLY` | SHARE UPDATE EXCLUSIVE | Nothing significant |
| `DROP INDEX` | ACCESS EXCLUSIVE | Everything |
| `DROP INDEX CONCURRENTLY` | SHARE UPDATE EXCLUSIVE | Nothing significant |

---

### AP-DB-005: Dual-Write Without Consistency

**Severity:** high

```python
# BAD: DB write + cache invalidation not atomic
def update_user(user_id, data):
    user = User.query.get(user_id)
    user.update(data)
    db.session.commit()
    
    # If this fails, cache has stale data!
    redis.delete(f"user:{user_id}")
```

**Fix — Cache-Aside Pattern (read-through):**
```python
# GOOD: Invalidate cache AFTER successful DB write
def update_user(user_id, data):
    user = User.query.get(user_id)
    user.update(data)
    db.session.commit()
    
    # Best-effort cache invalidation (failure is acceptable — cache will expire)
    try:
        redis.delete(f"user:{user_id}")
    except redis.RedisError as e:
        logger.warning("cache_invalidation_failed", user_id=user_id, error=str(e))
        # Cache will serve stale data until TTL expires — acceptable tradeoff
```

**Fix — Transactional Outbox (for DB + message queue):**
```python
# GOOD: Outbox pattern for DB + queue consistency
def update_user(user_id, data):
    with db.transaction():
        user = User.query.get(user_id)
        user.update(data)
        
        # Write event to outbox table (same transaction)
        outbox_event = OutboxEvent(
            event_type="user.updated",
            payload={"user_id": user_id, "changes": data},
            created_at=datetime.utcnow()
        )
        db.session.add(outbox_event)
        db.session.commit()
    
    # Separate process reads outbox and publishes to queue
    # (outbox_processor.py runs as a background job)
```

---

### AP-DB-006: Missing Resilience on DB Failure

**Severity:** high

```python
# BAD: DB failure = hard crash
def get_user(user_id):
    return User.query.get(user_id)
    # If DB is down: sqlalchemy.exc.OperationalError propagates to user
```

**Fix:**
```python
# GOOD: Graceful degradation with retry and fallback
from tenacity import retry, stop_after_attempt, wait_exponential
from sqlalchemy.exc import OperationalError, DisconnectionError

@retry(
    stop=stop_after_attempt(3),
    wait=wait_exponential(multiplier=1, min=1, max=10),
    retry=retry_if_exception_type((OperationalError, DisconnectionError))
)
def get_user_from_db(user_id):
    return User.query.get(user_id)

def get_user(user_id):
    # Try cache first
    cached = redis.get(f"user:{user_id}")
    if cached:
        return json.loads(cached)
    
    try:
        user = get_user_from_db(user_id)
        if user:
            # Cache for 5 minutes
            redis.setex(f"user:{user_id}", 300, json.dumps(user.to_dict()))
        return user
    except (OperationalError, DisconnectionError) as e:
        logger.error("db_unavailable", user_id=user_id, error=str(e))
        # Return None or raise ServiceUnavailableError
        raise ServiceUnavailableError("Database temporarily unavailable")
```

---

## Finding Templates

### HARDEN-DB-N-PLUS-ONE

```yaml
id: HARDEN-DB-N-PLUS-ONE
severity: high
category: database
location: "path/to/views.py:87"
description: "N+1 query: Post.author accessed in loop without select_related()."
impact: >
  With 100 posts, this generates 101 queries instead of 1-2. At scale (1000+ posts),
  this causes significant latency and database load. Under high traffic, can exhaust
  connection pool.
recommendation: >
  Add select_related('author') to the queryset:
  posts = Post.objects.select_related('author').all()
acceptance_criteria:
  - "Enable Django query logging; access /posts endpoint; verify query count is 1-2 (not N+1)"
  - "Response time for /posts with 100 posts is < 100ms (was > 1s)"
  - "Database query log shows single JOIN query, not N separate author queries"
verification_tier: 3
confidence: confirmed
assumptions: "Post.author is a ForeignKey (not ManyToMany)"
requires_human_review: false
```

### HARDEN-DB-LOCKING-MIGRATION

```yaml
id: HARDEN-DB-LOCKING-MIGRATION
severity: high
category: database
location: "migrations/0042_add_phone_column.py"
description: "ALTER TABLE users ADD COLUMN acquires ACCESS EXCLUSIVE lock."
impact: >
  On a table with 10M+ rows, this migration can take minutes. During that time,
  all reads and writes to the users table are blocked. This causes a production
  outage for all user-facing features.
recommendation: >
  Use expand/contract pattern:
  1. Add nullable column (fast in PG 11+)
  2. Deploy dual-write code
  3. Backfill in batches with pg_sleep between batches
  4. Deploy read-from-new-column code
  5. Drop old column in separate migration
acceptance_criteria:
  - "Migration runs on production-size dataset (10M rows) in < 1 second"
  - "Load test during migration shows no increase in error rate or latency"
  - "Migration has a corresponding down migration for rollback"
verification_tier: 4
confidence: confirmed
assumptions: "Table has 10M+ rows; smaller tables may be acceptable with brief maintenance window"
requires_human_review: true
```

---

## Query Performance Quick Reference

### Index Usage

```sql
-- Check if query uses index
EXPLAIN ANALYZE SELECT * FROM users WHERE email = 'alice@example.com';
-- Look for "Index Scan" vs "Seq Scan"

-- Find missing indexes (queries with Seq Scan on large tables)
SELECT schemaname, tablename, attname, n_distinct, correlation
FROM pg_stats
WHERE tablename = 'users'
ORDER BY n_distinct DESC;

-- Create index concurrently (no lock)
CREATE INDEX CONCURRENTLY idx_users_email ON users(email);
```

### Slow Query Logging

```sql
-- PostgreSQL: log queries slower than 1 second
ALTER SYSTEM SET log_min_duration_statement = 1000;
SELECT pg_reload_conf();

-- Or in postgresql.conf:
-- log_min_duration_statement = 1000  # milliseconds
```

### Connection Pool Monitoring

```python
# SQLAlchemy: log pool events
import logging
logging.getLogger('sqlalchemy.pool').setLevel(logging.DEBUG)

# Or use pool events
from sqlalchemy import event

@event.listens_for(engine, "connect")
def on_connect(dbapi_connection, connection_record):
    logger.info("db_connection_acquired", pool_size=engine.pool.size())

@event.listens_for(engine, "checkout")
def on_checkout(dbapi_connection, connection_record, connection_proxy):
    logger.debug("db_connection_checked_out", pool_size=engine.pool.size())
```

---

## ORM Coverage

<!-- axiom:trace work_item=hardening-skills-01 spec=hardening-database-axiom jira_ref=SWDE-7 plan=phase-2/task-1/step-fb001 -->

### ORMs with Explicit Patterns in This Skill

| ORM | Language | Eager-Load API | Section |
|---|---|---|---|
| **Django ORM** | Python | `select_related()`, `prefetch_related()` | AP-DB-001 |
| **SQLAlchemy** | Python | `joinedload()`, `subqueryload()` | AP-DB-001 |

### Extending Patterns to Other ORMs

The N+1 pattern is universal — the detection approach (loop over queryset, access related object) applies to all ORMs. The fix syntax differs by ORM.

| ORM | Language | Eager-Load API | Notes |
|---|---|---|---|
| **GORM** | Go | `Preload("Author")` | Equivalent to Django's `select_related`; use `db.Preload("Tags").Find(&posts)` for collections |
| **Prisma** | TypeScript/Node.js | `include: { author: true }` | Pass `include:` in `findMany()` / `findFirst()` queries |
| **TypeORM** | TypeScript/Node.js | `relations: ["author"]` in `find()` or `leftJoinAndSelect()` in QueryBuilder | Both approaches avoid N+1 |
| **Hibernate/JPA** | Java | `@EntityGraph` or `JOIN FETCH` in JPQL | Use `JOIN FETCH p.author` in JPQL or `@EntityGraph(attributePaths = {"author"})` on repository method |
| **ActiveRecord** | Ruby | `.includes(:author)` | Equivalent to `select_related`; use `.includes(:tags)` for associations |

### Additional Grep Patterns for Non-Python ORMs

```bash
# TypeScript/Prisma: N+1 risk — await inside loop
grep -rn "for.*await.*findMany\|for.*await.*findFirst" --include="*.ts" --include="*.js"

# Java/Hibernate: N+1 risk — entity access inside loop
grep -rn "for.*getEntity\|for.*\.get(" --include="*.java" | grep -v "Optional\|Map"
```

---

## Acceptance Criteria Templates (Tier 3+)

1. **N+1 eliminated**: Enable query logging → access endpoint → verify query count ≤ expected
2. **Connection pool bounded**: Generate load exceeding pool_size → verify requests fail with timeout error, not hang
3. **Transaction duration**: Log transaction duration → verify all complete within threshold (e.g., 100ms)
4. **Migration zero-downtime**: Run migration while load testing → verify no error rate increase
5. **DB failure graceful**: Kill DB connection → verify service returns degraded response, not 500

---

axiom:trace work_item=hardening-skills-01 spec=hardening-database-axiom jira_ref=SWDE-7 plan=phase-1/task-4/step-1
