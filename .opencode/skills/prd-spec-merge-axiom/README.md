# prd-spec-merge-axiom

Merge a finalized PRD (and its Spec-Merge Appendix) into the repo's `specs/` directory.

## Use

Load this skill when:
- A PRD has been finalized and stored in `.memory-bank/prds/`
- You need to create or update `specs/*.md` files to reflect the PRD's requirements
- You want to establish the L0 `prd → spec` trace link

## Workflow position

```
[prd-generator-axiom]
   ↓
.memory-bank/prds/<area>/<feature>.md  (PRD + Spec-Merge Appendix)
   ↓
[prd-spec-merge-axiom]  ← you are here
   ↓
specs/*.md  (updated/created with REQ-NNN blocks + axiom:trace prd=.memory-bank/prds/... markers)
   ↓
@pm-axiom → plan → @dev-axiom → ...
```

## Key outputs

1. Updated/new `specs/*.md` with REQ-NNN blocks, acceptance criteria, and `axiom:trace prd=.memory-bank/prds/...` markers
2. Updated `specs/README.md` and `.memory-bank/prds/_index.md`
3. Merge note at `.memory-bank/prds/<area>/<feature>-merge.md`

## Related

- `prd-generator-axiom/` — upstream PRD generator
- `.memory-bank/prds/` — canonical PRD storage
- `specs/21-Traceability-Doctrine.md` — traceability doctrine
