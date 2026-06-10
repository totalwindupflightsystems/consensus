import * as fs from "node:fs/promises";
import * as path from "node:path";
import { tool } from "@opencode-ai/plugin";

type QuestionOption = {
	key: string;
	label: string;
	tradeoffs?: string;
};

type Question = {
	id: string;
	question: string;
	options?: QuestionOption[];
	recommended?: string;
	spec_impact?: string;
};

function mustBeUnderWorktree(worktree: string, target: string): string {
	const resolvedWorktree = path.resolve(worktree);
	const resolvedTarget = path.resolve(target);
	const rel = path.relative(resolvedWorktree, resolvedTarget);
	if (rel.startsWith(`..${path.sep}`) || rel === "..") {
		throw new Error(`Refusing to write outside worktree: ${target}`);
	}
	return resolvedTarget;
}

function defaultQuestionsPath(workItemId?: string): string {
	if (workItemId && workItemId.trim().length > 0) {
		return path.join(
			".memory-bank",
			"work-items",
			workItemId.trim(),
			"spec-kickoff",
			"questions.yaml",
		);
	}
	return path.join("specs", "_inputs", "spec-kickoff-questions.yaml");
}

function toYamlString(value: string): string {
	// Minimal safe YAML scalar quoting.
	const escaped = value
		.replace(/\\/g, "\\\\")
		.replace(/\n/g, "\\n")
		.replace(/"/g, '\\"');
	return `"${escaped}"`;
}

export default tool({
	description:
		"Write a deterministic spec-kickoff question set to disk for users to answer quickly (numbered options + recommended defaults).",
	args: {
		work_item_id: tool.schema
			.string()
			.optional()
			.describe("Optional work item id for .memory-bank placement"),
		out_path: tool.schema
			.string()
			.optional()
			.describe(
				"Optional repo-relative output path; defaults to .memory-bank/... or specs/_inputs/...",
			),
		title: tool.schema
			.string()
			.describe("Human-readable title for the question set"),
		questions: tool.schema
			.array(
				tool.schema.object({
					id: tool.schema.string().describe("Stable question id, e.g. DEC-1"),
					question: tool.schema.string().describe("The question"),
					recommended: tool.schema
						.string()
						.optional()
						.describe("Recommended option key"),
					spec_impact: tool.schema
						.string()
						.optional()
						.describe("What spec sections this affects"),
					options: tool.schema
						.array(
							tool.schema.object({
								key: tool.schema.string().describe("Option key, e.g. A"),
								label: tool.schema.string().describe("Option label"),
								tradeoffs: tool.schema
									.string()
									.optional()
									.describe("Tradeoffs / implications"),
							}),
						)
						.optional(),
				}),
			)
			.describe("Questions to write"),
	},
	async execute(args, context) {
		const outPath = args.out_path?.trim().length
			? args.out_path.trim()
			: defaultQuestionsPath(args.work_item_id);

		// Enforce write under worktree.
		const abs = mustBeUnderWorktree(
			context.worktree,
			path.join(context.worktree, outPath),
		);
		await fs.mkdir(path.dirname(abs), { recursive: true });

		const now = new Date().toISOString().slice(0, 10);
		const lines: string[] = [];
		lines.push("---");
		lines.push("spec_kickoff_questions:");
		lines.push(`  title: ${toYamlString(args.title)}`);
		lines.push(`  created: ${toYamlString(now)}`);
		if (args.work_item_id?.trim()) {
			lines.push(`  work_item_id: ${toYamlString(args.work_item_id.trim())}`);
		}
		lines.push("  questions:");

		for (const q of args.questions as unknown as Question[]) {
			lines.push(`    - id: ${toYamlString(q.id)}`);
			lines.push(`      question: ${toYamlString(q.question)}`);
			if (q.recommended)
				lines.push(`      recommended: ${toYamlString(q.recommended)}`);
			if (q.spec_impact)
				lines.push(`      spec_impact: ${toYamlString(q.spec_impact)}`);
			if (q.options && q.options.length > 0) {
				lines.push("      options:");
				for (const opt of q.options) {
					lines.push(`        - key: ${toYamlString(opt.key)}`);
					lines.push(`          label: ${toYamlString(opt.label)}`);
					if (opt.tradeoffs)
						lines.push(`          tradeoffs: ${toYamlString(opt.tradeoffs)}`);
				}
			}
		}

		lines.push("---");
		lines.push("");
		lines.push("# How to answer");
		lines.push("# Reply in chat with: DEC-1=B, DEC-2=A, ...");
		lines.push(
			"# Or edit this file and add: answer: <key> under each question.",
		);

		await fs.writeFile(abs, lines.join("\n"), { encoding: "utf8" });
		return {
			out_path: outPath,
			bytes: Buffer.byteLength(lines.join("\n"), "utf8"),
		};
	},
});
