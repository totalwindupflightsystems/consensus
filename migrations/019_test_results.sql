-- 019_test_results.sql
-- Test results table for tool CI/CD verification (SPEC-010 §Tool Test Results).
-- Tracks automated test runs on agent-created tools.

CREATE TABLE test_results (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tool_id     UUID NOT NULL REFERENCES tools_registry(id),
    version     INT NOT NULL,
    passed      BOOLEAN NOT NULL,
    output      TEXT,
    run_at      TIMESTAMPTZ DEFAULT now()
);

-- Index for looking up test history per tool.
CREATE INDEX idx_test_results_tool ON test_results(tool_id, version DESC);
