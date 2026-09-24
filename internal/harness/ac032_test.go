// AC-032: Skill Routing — Lookup by name and linked tools
// Canonical from SPEC-010 §4, SPEC-011 §4.2.2
//
// Verifies that:
//   1. Skills can be looked up by name for full instructions
//   2. linked_tool_ids are stored and retrievable
//   3. Skills without metadata fall back to sensible defaults

package harness

import (
	"strings"
	"testing"
)

func TestAC032_SkillLookupByName(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert a skill
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-lu-1', 'excel_generator', '{"description":"Generate Excel files","when_to_use":"User requests spreadsheet"}', 'To generate Excel: 1. Call gen_excel() 2. Write to tool_files 3. Return path', 1)
	`)

	// Query by name
	rows, err := th.conn.Query(th.ctx, `SELECT id, name, instructions, metadata FROM skills_registry WHERE name = $1`, "excel_generator")
	if err != nil {
		t.Fatalf("AC-032: query skill: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("AC-032: skill not found by name")
	}

	instr := toString(rows[0]["instructions"])
	if instr != "To generate Excel: 1. Call gen_excel() 2. Write to tool_files 3. Return path" {
		t.Errorf("AC-032: instructions = %q, want full instruction text", instr)
	}

	name := toString(rows[0]["name"])
	if name != "excel_generator" {
		t.Errorf("AC-032: name = %q, want 'excel_generator'", name)
	}
	t.Log("AC-032 PASS: skill lookup by name returns full instructions")
}

func TestAC032_SkillLinkedTools(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert a skill with linked tool IDs (as JSON array in TEXT column for SQLite)
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, linked_tool_ids, enabled)
		VALUES ('sk-tl-1', 'web_automation', '{"description":"Browser automation","when_to_use":"User needs web data"}', 'Use the scraper tool', '["tool-fetch","tool-parse"]', 1)
	`)

	// Query linked_tool_ids
	rows, err := th.conn.Query(th.ctx, `SELECT linked_tool_ids FROM skills_registry WHERE name = $1`, "web_automation")
	if err != nil {
		t.Fatalf("AC-032: query linked_tool_ids: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("AC-032: skill not found")
	}

	toolIDs := toString(rows[0]["linked_tool_ids"])
	if !strings.Contains(toolIDs, "tool-fetch") || !strings.Contains(toolIDs, "tool-parse") {
		t.Errorf("AC-032: linked_tool_ids = %q, should contain both tool refs", toolIDs)
	}
	t.Logf("AC-032: linked_tool_ids = %s", toolIDs)
	t.Log("AC-032 PASS: skill linked_tool_ids stored and retrievable")
}

func TestAC032_SkillMissingMetadataFallback(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert a skill with minimal metadata
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-mf-1', 'custom_script', '{}', 'Run the custom script with parameters', 1)
	`)

	skills, err := th.Harness.readSkillsMetadata(th.ctx)
	if err != nil {
		t.Fatalf("AC-032: readSkillsMetadata: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("AC-032: expected 1 skill, got %d", len(skills))
	}

	// When metadata has no description, the fallback is the name itself
	if skills[0].Description == "" {
		t.Errorf("AC-032: description should have fallback, got empty")
	}
	if skills[0].WhenToUse != "See full instructions" {
		t.Errorf("AC-032: when_to_use = %q, want 'See full instructions'", skills[0].WhenToUse)
	}
	t.Logf("AC-032: fallback description = %q, when_to_use = %q", skills[0].Description, skills[0].WhenToUse)
	t.Log("AC-032 PASS: minimal metadata falls back to sensible defaults")
}
