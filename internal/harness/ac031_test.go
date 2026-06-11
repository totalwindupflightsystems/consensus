// AC-031: Skill Loading into Context
// Canonical from SPEC-012 §2.1 (Layer 2: Skills metadata), SPEC-010 §4
//
// Verifies that:
//   1. readSkillsMetadata returns skills from skills_registry
//   2. Metadata fields (description, when_to_use) are extracted from JSON
//   3. Only enabled skills are returned
//   4. Skills are sorted alphabetically by name

package harness

import (
	"testing"
)

func TestAC031_ReadSkillsMetadata_Basic(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert skills with metadata
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-ctx-1', 'data_analyzer', '{"description":"Analyze datasets","when_to_use":"When user has CSV data","version":"2.0"}', 'Run analysis queries', 1)
	`)
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-ctx-2', 'web_scraper', '{"description":"Scrape web pages","when_to_use":"When user asks for data extraction","version":"1.5"}', 'Use fetch tool', 1)
	`)

	// Read skills metadata via harness
	skills, err := th.Harness.readSkillsMetadata(th.ctx)
	if err != nil {
		t.Fatalf("AC-031: readSkillsMetadata: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("AC-031: expected 2 skills, got %d", len(skills))
	}

	// Check data_analyzer was parsed correctly
	foundData := false
	foundWeb := false
	for _, s := range skills {
		switch s.Name {
		case "data_analyzer":
			foundData = true
			if s.Description != "Analyze datasets" {
				t.Errorf("AC-031: description = %q, want 'Analyze datasets'", s.Description)
			}
			if s.WhenToUse != "When user has CSV data" {
				t.Errorf("AC-031: when_to_use = %q, want 'When user has CSV data'", s.WhenToUse)
			}
			if s.Version != "2.0" {
				t.Errorf("AC-031: version = %q, want '2.0'", s.Version)
			}
		case "web_scraper":
			foundWeb = true
			if s.Description != "Scrape web pages" {
				t.Errorf("AC-031: description = %q, want 'Scrape web pages'", s.Description)
			}
			if s.WhenToUse != "When user asks for data extraction" {
				t.Errorf("AC-031: when_to_use = %q, want 'When user asks for data extraction'", s.WhenToUse)
			}
		}
	}
	if !foundData {
		t.Error("AC-031: data_analyzer not in skills list")
	}
	if !foundWeb {
		t.Error("AC-031: web_scraper not in skills list")
	}
	t.Log("AC-031 PASS: skills metadata loaded and parsed correctly")
}

func TestAC031_ReadSkillsMetadata_OnlyEnabled(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert one enabled and one disabled skill
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-en-1', 'enabled_skill', '{"description":"Active skill"}', 'do stuff', 1)
	`)
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-dis-1', 'disabled_skill', '{"description":"Inactive skill"}', 'secret stuff', 0)
	`)

	skills, err := th.Harness.readSkillsMetadata(th.ctx)
	if err != nil {
		t.Fatalf("AC-031: readSkillsMetadata: %v", err)
	}

	// Should only return enabled skill
	if len(skills) != 1 {
		t.Fatalf("AC-031: expected 1 enabled skill, got %d", len(skills))
	}
	if skills[0].Name != "enabled_skill" {
		t.Errorf("AC-031: skill = %q, want 'enabled_skill'", skills[0].Name)
	}
	t.Log("AC-031 PASS: disabled skills excluded from context")
}

func TestAC031_ReadSkillsMetadata_AlphabeticalOrder(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Insert skills in non-alphabetical order
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-zb', 'zebra_skill', '{"description":"Last"}', 'z', 1)
	`)
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-al', 'alpha_skill', '{"description":"First"}', 'a', 1)
	`)
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-md', 'middle_skill', '{"description":"Middle"}', 'm', 1)
	`)

	skills, err := th.Harness.readSkillsMetadata(th.ctx)
	if err != nil {
		t.Fatalf("AC-031: readSkillsMetadata: %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("AC-031: expected 3 skills, got %d", len(skills))
	}

	// Verify alphabetical order (SQL query uses ORDER BY name)
	if skills[0].Name != "alpha_skill" {
		t.Errorf("AC-031: first = %q, want 'alpha_skill'", skills[0].Name)
	}
	if skills[1].Name != "middle_skill" {
		t.Errorf("AC-031: second = %q, want 'middle_skill'", skills[1].Name)
	}
	if skills[2].Name != "zebra_skill" {
		t.Errorf("AC-031: third = %q, want 'zebra_skill'", skills[2].Name)
	}
	t.Log("AC-031 PASS: skills returned in alphabetical order")
}
