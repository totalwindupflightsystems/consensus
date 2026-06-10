/**
 * Axiom Compaction Hook Plugin
 *
 * Reads `.opencode/prompts/compaction.md` and injects it as context
 * during session compaction. This lets operators customize what survives
 * compaction by editing a Markdown file rather than modifying TypeScript.
 *
 * This is a lightweight local plugin that complements the main Axiom
 * plugin at `.axiom/plugin/src/hooks.ts` (which injects dynamic state
 * like maturity level and active work item).
 *
 * Hook: experimental.session.compacting
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-031
 * Best practice: .memory-bank/best-practices/opencode-plugin-compaction-hooks.md
 *
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md plan=phase-70-1/task-70-1-5
 */

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

export const CodeOpsCompactionPlugin = async ({ directory }) => {
	return {
		tool: {},
		"experimental.session.compacting": async (_input, output) => {
			try {
				// Pattern 1: File-based compaction context
				// Read the custom compaction prompt and inject it as context
				const compactionFile = join(
					directory,
					".opencode",
					"prompts",
					"compaction.md",
				);
				if (existsSync(compactionFile)) {
					const content = readFileSync(compactionFile, "utf-8");
					// Only inject if content is non-empty and reasonable size
					// Keep under ~2000 chars to respect LLM token budget
					if (content.trim().length > 0 && content.length < 10000) {
						output.context.push(content);
					}
				}

				// Pattern 2: Dynamic active work item injection
				// Complement the static file with live work item state
				const currentPath = join(
					directory,
					".memory-bank",
					"work-items",
					"_current.md",
				);
				if (existsSync(currentPath)) {
					try {
						const currentContent = readFileSync(currentPath, "utf-8");
						const activeLine = currentContent
							.split("\n")
							.find((l) => l.includes("Active") || l.includes("work_item"));
						if (activeLine) {
							output.context.push(
								`Active Axiom work item: ${activeLine.trim()}`,
							);
						}
					} catch {
						// Ignore read errors for optional file
					}
				}

				// REQ-PLG-031: do NOT replace the default compaction prompt
				// output.prompt remains undefined — we only append context
			} catch {
				// REQ-PLG-037: non-blocking — swallow errors silently
				// A crash here would break the session
			}
		},
	};
};
