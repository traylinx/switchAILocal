# Skill Examples

Real-world examples of well-structured skills for reference.

## Example 1: Technical Expert Skill

A skill for domain-specific technical expertise.

```markdown
---
name: go-expert
description: Specialized knowledge for writing professional, idiomatic Go code.
required-capability: coding
---

# Go Expert

You are a Senior Go Engineer specializing in high-performance systems.

## Code Style
- **Errors**: Use `fmt.Errorf("context: %w", err)` for wrapping
- **Concurrency**: Use `sync.Mutex` for shared state
- **Logging**: Use the internal logger via `log.Infof`

## Project Structure
- `internal/`: Private implementation
- `plugins/`: Extensions
- `sdk/`: Public libraries

When asked to write Go code, ensure it compiles and handles errors properly.
```

**Why it works:**
- Clear, concise persona
- Specific conventions (not generic advice)
- Actionable guidelines

## Example 2: Vision-Based Skill

A skill that analyzes visual content.

```markdown
---
name: vision-expert
description: Expert in analyzing images, diagrams, and UI screenshots.
required-capability: vision
---

# Vision Expert

You are a multimodal AI assistant specialized in visual analysis.

## Capabilities
- **UI Analysis**: Describe layouts, components, design patterns
- **Code from Image**: Convert screenshots to HTML/CSS/React
- **Diagram Interpretation**: Explain architecture diagrams

## Guidelines
- Be precise and detailed
- If generating code from UI, use clean semantics
- If unclear, ask for clarification
```

**Why it works:**
- Correct capability (`vision`) for image tasks
- Clear scope of what it can do
- Practical guidelines

## Example 3: Workflow-Based Skill

A skill with multi-step processes.

```markdown
---
name: mcp-builder
description: Guide for building Model Context Protocol (MCP) servers.
required-capability: coding
---

# MCP Builder

## Workflow

1. **Define Tools**: Specify tool schemas in `tools.json`
2. **Implement Handlers**: Create handler functions
3. **Test Locally**: Run `npm run dev` and test with MCP client
4. **Package**: Build and distribute

## Tool Schema Format

```json
{
  "name": "tool_name",
  "description": "What the tool does",
  "inputSchema": {
    "type": "object",
    "properties": { ... }
  }
}
```

## References
- See `references/mcp-spec.md` for protocol details
- See `references/examples.md` for sample implementations
```

**Why it works:**
- Clear sequential workflow
- Concrete examples
- References for deep dives

## Example 4: Skill with Scripts

A skill that bundles executable scripts.

```markdown
---
name: pdf-processor
description: Process, edit, and analyze PDF documents.
required-capability: coding
---

# PDF Processor

## Quick Start

Extract text from a PDF:
```bash
python scripts/extract_text.py input.pdf
```

## Available Scripts

| Script | Purpose |
|--------|---------|
| `extract_text.py` | Extract text content |
| `merge_pdfs.py` | Combine multiple PDFs |
| `split_pdf.py` | Split into pages |
| `fill_form.py` | Fill form fields |

## Usage Examples

**Merge PDFs:**
```bash
python scripts/merge_pdfs.py file1.pdf file2.pdf -o merged.pdf
```

**Fill a form:**
```bash
python scripts/fill_form.py template.pdf data.json -o filled.pdf
```
```

**Why it works:**
- Scripts for deterministic operations
- Clear documentation of each script
- Practical usage examples

## Example 5: Reference-Heavy Skill

A skill with extensive reference documentation.

```markdown
---
name: bigquery-analyst
description: Query and analyze data in BigQuery with company-specific schemas.
required-capability: reasoning
---

# BigQuery Analyst

## Quick Reference

Common tables:
- `analytics.events` - User events
- `sales.opportunities` - Sales pipeline
- `finance.revenue` - Revenue data

## Schema References

For detailed schemas, see:
- `references/analytics.md` - Analytics tables
- `references/sales.md` - Sales tables
- `references/finance.md` - Finance tables

## Query Patterns

**Daily active users:**
```sql
SELECT DATE(timestamp) as date, COUNT(DISTINCT user_id) as dau
FROM analytics.events
WHERE timestamp >= DATE_SUB(CURRENT_DATE(), INTERVAL 30 DAY)
GROUP BY date
ORDER BY date
```

Load the appropriate reference file based on the user's query domain.
```

**Why it works:**
- Quick reference in main file
- Detailed schemas in separate files
- Progressive disclosure

## Anti-Patterns to Avoid

### ❌ Too Verbose

```markdown
# Bad: Explaining obvious things
This skill helps you write code. Code is a set of instructions...
```

### ❌ Too Generic

```markdown
# Bad: No specific guidance
Be helpful and write good code.
```

### ❌ Missing Capability

```markdown
# Bad: Vision skill without vision capability
name: ui-analyzer
description: Analyze UI screenshots
# Missing: required-capability: vision
```

### ❌ Unnecessary Files

```markdown
skill/
├── SKILL.md
├── README.md          # ❌ Unnecessary
├── CHANGELOG.md       # ❌ Unnecessary
├── INSTALLATION.md    # ❌ Unnecessary
└── scripts/
```

## Checklist for New Skills

- [ ] Clear, descriptive `name` in kebab-case
- [ ] Comprehensive `description` explaining WHEN to use
- [ ] Correct `required-capability` for the task type
- [ ] Concise instructions (< 500 lines)
- [ ] Specific, actionable guidance
- [ ] Examples where helpful
- [ ] Scripts for repetitive/deterministic tasks
- [ ] References for detailed documentation
- [ ] No unnecessary files
