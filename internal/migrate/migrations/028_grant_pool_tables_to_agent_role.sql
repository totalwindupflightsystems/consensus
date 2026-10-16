-- ============================================================================
-- Conscience — 028_grant_pool_tables_to_agent_role.sql
-- ============================================================================
-- PG-GRANT-42501: the application pool runs EVERY connection under
-- SET ROLE agent_role (internal/db/postgres/postgres.go, AfterConnect hook), but
-- several migrations created tables the pool then queried without ever granting
-- them to that role — so those statements fail with SQLSTATE 42501
-- ("permission denied for table ...").
--
-- Reported failure (live serve log, local PG probe 2026-10-15):
--   webhook: fetch pending events: permission denied for table external_events
-- Corroborating failures from the same probe (same defect class):
--   permission denied for table hitl_configuration
--   permission denied for table api_keys   (fixed separately, PG-AUTH-001, by
--                                            routing the auth lookup through the
--                                            admin pool — see "api_keys" below)
--
-- The grant set below is DERIVED FROM CODE, not from the migration numbering:
-- a census of every SQL literal in internal/** and cmd/** (non-test) that is
-- issued through a non-admin handle (s.db / h.db / m.database / w.db /
-- e.database / r.db / s.database — i.e. anything that is not
-- admindb / AdminDB / *pgpostgres.DB.AdminDB() / the migration runner's own
-- adminDB). For each table the census yields the exact verbs the pool executes;
-- this migration grants precisely those verbs and nothing more.
--
--   table                   verbs granted        evidence
--   ----------------------  -------------------  ---------------------------------------------
--   external_events         SELECT INSERT UPDATE webhook.go poller SELECT/INSERT/UPDATE lifecycle
--   webhook_registrations   SELECT INS UPD DEL   webhook.go CRUD (list/get/upsert/delete/disable)
--   routing_rules           SELECT               webhook.go loadRoutingRules
--   approval_requests       SELECT INSERT UPDATE hitl.go + tools/approval.go + shim approve/deny
--   hitl_configuration      SELECT INSERT DELETE hitl.go upsert (DELETE+INSERT, portable form)
--   notification_log        INSERT               hitl.go notification audit insert
--   api_keys                SELECT INSERT UPDATE shim session-key mint/lookup, billing list/revoke
--   shim_session_map        INSERT               shim session create (external->consensus map)
--   agent_circuit_breakers  SELECT INSERT UPDATE harness/circuit.go upsert ON CONFLICT DO UPDATE
--   projects                SELECT               shim GET project list
--   event_embeddings        SELECT               memory/retrieval.go semantic retrieval JOIN
--   audit_logs              SELECT INSERT        harness/audit.go insert; api/service+events read
--   compression_queue       SELECT UPDATE        compression worker fetch/claim/complete/fail
--   system_settings         SELECT INS UPD DEL   shim + api/service settings CRUD (upsert form)
--   tool_results            SELECT INSERT        harness/tool_executor.go write; api LEFT JOIN read
--   schema_versions         SELECT               shim reads applied_at (runner writes via adminDB)
--   external_quarantine     + UPDATE             quarantine triage transitions (001 had S,I only)
--   iteration_commits       + UPDATE             harness/audit.go update llm_response (001 had S,I)
--   tool_requests           + UPDATE             harness/tool_executor.go status transitions (001 S,I)
--   model_registry          + INSERT UPDATE      modelsync syncer writes (001 had SELECT only)
--
-- Deliberately NOT granted (so this migration does not silently widen the
-- security model):
--   * memory_events UPDATE/DELETE — 001 REVOKEs them on purpose (append-only
--     ledger; SPEC-002/SPEC-005 + the "UPDATE/DELETE is REJECTED by the database
--     kernel" prompt contract). The compression worker's
--     `UPDATE memory_events SET summary_text` currently runs on this same
--     agent_role pool and still needs a dedicated role/pool (or a column-level
--     grant) — that is a SEPARATE defect, and re-granting UPDATE here would
--     break append-only enforcement for every agent.
--   * api_rate_limits — only ever queried through s.admindb (api/server.go).
--   * agent_budget_limits, test_results, workflows, custom_agent_tools,
--     tool_files, agent_messages, secret_access_audit, memory_pages INSERT,
--     display_modes-adjacent audit tables — no SQL in non-test code targets
--     them, so no pool privilege is required.
--   * dynamic_table_schemas — created by the legacy top-level
--     migrations/009_json_schema_support.sql, which is NOT in the embedded
--     migration set; granting it here would fail on a fresh database. (Its
--     absence from the embedded set is a separate finding.)
--
-- Idempotency: GRANT is naturally idempotent in PostgreSQL (re-granting an
-- existing privilege is a no-op), and the pg→SQLite filter strips GRANT
-- statements (internal/migrate/migrate.go filterForSQLite), so this migration is
-- a no-op on SQLite and safe to re-run on Postgres.
--
-- axiom:trace work_item=PG-GRANT-42501
--   spec=specs/003-database.md,specs/005-security.md,specs/013-webhooks-and-events.md
--   impl=internal/migrate/migrations/028_grant_pool_tables_to_agent_role.sql
-- ============================================================================

BEGIN;

-- ---------------------------------------------------------------------------
-- 007 — webhook ingestion + routing (the reported 42501).
-- ---------------------------------------------------------------------------
GRANT SELECT, INSERT, UPDATE ON external_events TO agent_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON webhook_registrations TO agent_role;
GRANT SELECT ON routing_rules TO agent_role;

-- ---------------------------------------------------------------------------
-- 008 — Human-in-the-Loop interrupt state.
-- ---------------------------------------------------------------------------
GRANT SELECT, INSERT, UPDATE ON approval_requests TO agent_role;
GRANT SELECT, INSERT, DELETE ON hitl_configuration TO agent_role;
GRANT INSERT ON notification_log TO agent_role;

-- ---------------------------------------------------------------------------
-- 002 — protocol-shim session map + API key store.
-- ---------------------------------------------------------------------------
GRANT SELECT, INSERT, UPDATE ON api_keys TO agent_role;
GRANT INSERT ON shim_session_map TO agent_role;

-- ---------------------------------------------------------------------------
-- 003 — per-session circuit breakers (harness upsert needs UPDATE).
-- ---------------------------------------------------------------------------
GRANT SELECT, INSERT, UPDATE ON agent_circuit_breakers TO agent_role;

-- ---------------------------------------------------------------------------
-- 015/016 — scope model + embedding store.
-- ---------------------------------------------------------------------------
GRANT SELECT ON projects TO agent_role;
GRANT SELECT ON event_embeddings TO agent_role;

-- ---------------------------------------------------------------------------
-- Tables created by 001 that the pool queries but that were never granted.
-- ---------------------------------------------------------------------------
GRANT SELECT, INSERT ON audit_logs TO agent_role;
GRANT SELECT, UPDATE ON compression_queue TO agent_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON system_settings TO agent_role;
GRANT SELECT, INSERT ON tool_results TO agent_role;
GRANT SELECT ON schema_versions TO agent_role;

-- ---------------------------------------------------------------------------
-- Existing 001/004 grants that were missing a verb the pool actually executes
-- (a partial grant still raises 42501 on the missing verb).
-- ---------------------------------------------------------------------------
GRANT UPDATE ON external_quarantine TO agent_role;
GRANT UPDATE ON iteration_commits TO agent_role;
GRANT UPDATE ON tool_requests TO agent_role;
GRANT INSERT, UPDATE ON model_registry TO agent_role;

COMMIT;
