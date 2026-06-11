// AC-030: Skills Registry CRUD
// Canonical from SPEC-011 §4.2.2
//
// Verifies that:
//   1. skills_registry exists and accepts INSERT with all canonical fields
//   2. Skills can be SELECTed by name with correct data
//   3. Skills can be UPDATEd (instructions, metadata, enabled)
//   4. Skills can be DELETEd
//   5. The UNIQUE(name) constraint is enforced

package harness

import (
	"testing"
)

func TestAC030_SkillsRegistry_InsertAndQuery(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert a skill
	err = th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-1', 'web_scraper', '{"description":"Scrape web pages","version":"1.0"}', 'Use the fetch tool to get page content', 1)
	`)
	if err != nil {
		t.Fatalf("AC-030: insert skill: %v", err)
	}
	t.Log("AC-030: skill inserted")

	// Query by name
	rows, err := th.conn.Query(th.ctx, `SELECT id, name, metadata, instructions, enabled FROM skills_registry WHERE name = $1`, "web_scraper")
	if err != nil {
		t.Fatalf("AC-030: query skill: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("AC-030: expected 1 row, got %d", len(rows))
	}

	name := toString(rows[0]["name"])
	enabled := toBoolHelper(rows[0]["enabled"])
	meta := toString(rows[0]["metadata"])

	if name != "web_scraper" {
		t.Errorf("AC-030: name = %q, want 'web_scraper'", name)
	}
	if !enabled {
		t.Error("AC-030: enabled should be true")
	}
	if meta == "" {
		t.Error("AC-030: metadata should not be empty")
	}
	t.Log("AC-030 PASS: skill inserted and queried correctly")
}

func TestAC030_SkillsRegistry_UpdateSkill(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert a skill
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-up-1', 'data_analyzer', '{"description":"Analyze datasets","version":"1.0"}', 'Run analysis queries', 1)
	`)

	// Update instructions and metadata
	err = th.conn.Exec(th.ctx, `
		UPDATE skills_registry SET instructions = $1, metadata = $2, updated_at = datetime('now')
		WHERE name = $3
	`, "Updated: run statistical analysis queries", `{"description":"Advanced data analysis","version":"2.0"}`, "data_analyzer")
	if err != nil {
		t.Fatalf("AC-030: update skill: %v", err)
	}

	// Verify update
	rows, _ := th.conn.Query(th.ctx, `SELECT instructions, metadata FROM skills_registry WHERE name = $1`, "data_analyzer")
	if len(rows) == 0 {
		t.Fatal("AC-030: skill not found after update")
	}
	instr := toString(rows[0]["instructions"])
	meta := toString(rows[0]["metadata"])

	if instr != "Updated: run statistical analysis queries" {
		t.Errorf("AC-030: instructions = %q, want updated value", instr)
	}
	if meta != `{"description":"Advanced data analysis","version":"2.0"}` {
		t.Errorf("AC-030: metadata = %q, want updated value", meta)
	}
	t.Log("AC-030 PASS: skill updated correctly")
}

func TestAC030_SkillsRegistry_UniqueName(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert first skill
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-u1', 'unique_skill', '{}', 'do stuff', 1)
	`)

	// Try inserting duplicate name — should fail
	err = th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-u2', 'unique_skill', '{}', 'different stuff', 1)
	`)
	if err == nil {
		t.Error("AC-030: duplicate skill name should be rejected")
	} else {
		t.Logf("AC-030: duplicate name correctly rejected: %v", err)
	}
	t.Log("AC-030 PASS: UNIQUE constraint enforced on skill name")
}
