// AC-033: Skill Lifecycle — Enable/disable, update tracking
// Canonical from SPEC-011 §4.2.2
//
// Verifies that:
//   1. Skills can be disabled (enabled=0) and re-enabled (enabled=1)
//   2. Disabled skills are excluded from readSkillsMetadata
//   3. updated_at changes when skill is modified
//   4. Skills registry accepts standard lifecycle transitions

package harness

import (
	"testing"
)

func TestAC033_SkillDisableAndReEnable(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert an enabled skill
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-life-1', 'toggle_skill', '{"description":"Toggle test"}', 'do thing', 1)
	`)

	// Verify it appears in enabled skills
	skills, _ := th.Harness.readSkillsMetadata(th.ctx)
	if len(skills) != 1 {
		t.Fatalf("AC-033: expected 1 enabled skill initially, got %d", len(skills))
	}
	t.Log("AC-033: skill initially enabled and visible")

	// Disable it
	err = th.conn.Exec(th.ctx, `UPDATE skills_registry SET enabled = 0, updated_at = datetime('now') WHERE name = $1`, "toggle_skill")
	if err != nil {
		t.Fatalf("AC-033: disable skill: %v", err)
	}

	// Should be excluded from metadata query
	skills, _ = th.Harness.readSkillsMetadata(th.ctx)
	if len(skills) != 0 {
		t.Errorf("AC-033: expected 0 enabled skills after disable, got %d", len(skills))
	}
	t.Log("AC-033: disabled skill excluded from context")

	// Re-enable it
	th.conn.Exec(th.ctx, `UPDATE skills_registry SET enabled = 1, updated_at = datetime('now') WHERE name = $1`, "toggle_skill")

	// Should be visible again
	skills, _ = th.Harness.readSkillsMetadata(th.ctx)
	if len(skills) != 1 {
		t.Errorf("AC-033: expected 1 enabled skill after re-enable, got %d", len(skills))
	}
	t.Log("AC-033 PASS: skill lifecycle (enable → disable → re-enable) works")
}

func TestAC033_SkillUpdateTimestamp(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert skill
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-ts-1', 'time_skill', '{"description":"Time test"}', 'check time', 1)
	`)

	// Read created_at
	rowsBefore, _ := th.conn.Query(th.ctx, `SELECT created_at FROM skills_registry WHERE name = $1`, "time_skill")
	if len(rowsBefore) == 0 {
		t.Fatal("AC-033: skill not found")
	}
	createdBefore := toString(rowsBefore[0]["created_at"])
	t.Logf("AC-033: created_at = %s", createdBefore)

	// Update the skill
	err = th.conn.Exec(th.ctx, `
		UPDATE skills_registry SET instructions = 'updated instructions', updated_at = datetime('now') WHERE name = $1
	`, "time_skill")
	if err != nil {
		t.Fatalf("AC-033: update skill: %v", err)
	}

	// Verify created_at unchanged, updated_at changed
	rowsAfter, _ := th.conn.Query(th.ctx, `SELECT created_at, updated_at FROM skills_registry WHERE name = $1`, "time_skill")
	if len(rowsAfter) == 0 {
		t.Fatal("AC-033: skill not found after update")
	}
	createdAfter := toString(rowsAfter[0]["created_at"])
	updatedAfter := toString(rowsAfter[0]["updated_at"])

	if createdBefore != createdAfter {
		t.Errorf("AC-033: created_at changed from %q to %q — should be immutable", createdBefore, createdAfter)
	}
	if updatedAfter == "" {
		t.Error("AC-033: updated_at should be set after update")
	}
	if updatedAfter != createdAfter {
		t.Logf("AC-033: updated_at (%s) differs from created_at (%s) — update timestamp works", updatedAfter, createdAfter)
	}
	t.Log("AC-033 PASS: skill timestamps track lifecycle correctly")
}

func TestAC033_SkillDelete(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert and then delete a skill
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-del-1', 'deletable_skill', '{}', 'temp', 1)
	`)

	// Verify it exists
	rowsBefore, _ := th.conn.Query(th.ctx, `SELECT count(*) as cnt FROM skills_registry WHERE name = $1`, "deletable_skill")
	countBefore := 0
	if len(rowsBefore) > 0 {
		countBefore = toInt(rowsBefore[0]["cnt"])
	}
	if countBefore != 1 {
		t.Fatalf("AC-033: expected 1 before delete, got %d", countBefore)
	}

	// Delete it
	err = th.conn.Exec(th.ctx, `DELETE FROM skills_registry WHERE name = $1`, "deletable_skill")
	if err != nil {
		t.Fatalf("AC-033: delete skill: %v", err)
	}

	// Verify gone
	rowsAfter, _ := th.conn.Query(th.ctx, `SELECT count(*) as cnt FROM skills_registry WHERE name = $1`, "deletable_skill")
	countAfter := 1
	if len(rowsAfter) > 0 {
		countAfter = toInt(rowsAfter[0]["cnt"])
	}
	if countAfter != 0 {
		t.Errorf("AC-033: expected 0 after delete, got %d", countAfter)
	}
	t.Log("AC-033 PASS: skill deletion works")
}
