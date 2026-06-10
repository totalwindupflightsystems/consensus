# Hypercognitive Skill Compiler - References

This directory contains reference documentation for the hypercognitive skill compiler.

## Files

### 1. `agent-skills-spec.md`
- Agent Skills specification requirements
- Validation rules for skill components
- Best practices and patterns
- Quality gates and error handling

## Using These References

The skill compiler should reference these documents when:
- Validating skill requirements against the specification
- Checking compliance with Agent Skills standards
- Implementing best practices in generated skills
- Handling edge cases and error scenarios

## Integration with Compiler

These references support the internal thinking modes of the compiler:

### For `mode_constraints_inventory`
- Hard constraints from Agent Skills spec
- Validation rules for name, description, structure

### For `mode_quality_gates_design`
- Pre/during/post-flight validation criteria
- Compliance checklists

### For `mode_error_taxonomy`
- Common error categories in skill creation
- Recovery strategies for spec violations

## Extending References

Add new reference files when:
- New Agent Skills specification versions are released
- Additional compatibility requirements emerge
- New best practices are established
- Additional error scenarios are identified

## Version Compatibility

This compiler is designed for:
- Agent Skills specification v1.0+
- OpenCode compatibility
- Skills CLI compatibility (npx skills)

## Related Skills in This Repository

### For Basic Skill Creation
- **skill-builder**: Structured templates and validation tools for basic to intermediate skill creation.

### For Complex Skill Creation
- **hypercognitive-skill-compiler**: This skill - full hypercognitive compiler preserving all original thinking modes, artifact registry, and compilation passes.

## Related Resources
- [Agent Skills Specification](https://agentskills.io)
- [OpenCode Skills Documentation](https://opencode.ai/docs/skills/)
- [Skills CLI](https://github.com/vercel-labs/skills)