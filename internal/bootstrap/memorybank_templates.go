// Memory-bank skeleton templates bootstrapped by `consensus init`.
//
// The skeleton (.memory-bank/_index.md, _prompt.md, findings/) is COMMITTED
// (C-GAP-017) so a fresh clone satisfies AGENTS.md "Required Reading";
// these constants keep `consensus init` output byte-identical to the
// committed skeleton (edit both together). work-items/ stays gitignored —
// per-item plans are local agent memory. Content mirrors what AGENTS.md
// promises agents: _index.md navigates the bank, _prompt.md gives the
// rules, findings/_index.md is the finding index, work-items/ holds
// per-item plans.
//
// axiom:trace work_item=c-gap-013 spec=specs/016-cli-interface.md impl=internal/bootstrap/memorybank_templates.go test=internal/bootstrap/memory_bank_test.go
package bootstrap

// memoryBankIndexTemplate is the .memory-bank/_index.md placeholder:
// the memory inventory agents are directed to read at task start
// (AGENTS.md "Required Reading").
const memoryBankIndexTemplate = `# Memory Bank — Index

This directory is the Consensus project's long-term agent memory
(AGENTS.md: "Long-term project memory lives in .memory-bank/"). The
skeleton (this file, _prompt.md, findings/) is committed so a fresh clone
has it; agents create and maintain the contents during their work.
work-items/ is gitignored and bootstrapped by ` + "`consensus init`" + `.

## Navigation

- findings/_index.md — findings, patterns, and self-improvement notes
- work-items/<WORK_ITEM_ID>/ — per work-item plans and evidence
- _prompt.md — rules for using this memory bank

See AGENTS.md for the agent rules that reference this directory.
`

// memoryBankPromptTemplate is the .memory-bank/_prompt.md placeholder:
// the rules agents must follow when working in the memory bank
// (AGENTS.md "Memory Bank").
const memoryBankPromptTemplate = `# Memory Bank — Prompt

Rules for agents using this memory bank (see AGENTS.md "Memory Bank"):

1. Use _index.md to navigate: it is the memory inventory.
2. Write findings to findings/ — never into AGENTS.md.
3. Each finding type has its own subfolder with _index.md and _prompt.md.
4. For work items, use work-items/<WORK_ITEM_ID>/.
5. Never write secrets to the memory bank.
6. The skeleton is committed; work-items/ and other local contents are
   gitignored — agent memory beyond the skeleton is local, not source.
`

// memoryBankFindingsIndexTemplate is the .memory-bank/findings/_index.md
// placeholder: the finding index agents are directed to consult before
// writing a finding (AGENTS.md "Findings & Self-Improvement").
const memoryBankFindingsIndexTemplate = `# Findings — Index

Agents accumulate findings, patterns, and self-improvement notes here
(AGENTS.md "Findings & Self-Improvement").

This index is part of the committed skeleton (mirrored by ` + "`consensus init`" + `).
Add a section per finding type below, each with its own subfolder containing
_index.md and _prompt.md, and link it here.

- (no findings recorded yet)
`
