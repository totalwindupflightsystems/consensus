---
name: intake-processing
description: >
  Loadable skill for multi-modal file intake and document processing.
  Provides trigger conditions, parameter guidance, processor selection,
  security constraints, and error handling for the intake_process_file tool.
version: "1.0"
license: MIT
compatibility: opencode
metadata:
  workflow: intake
  outputs: "intake_process_file tool calls"
  aliases: ["intake", "document-processing", "multi-modal", "file-extraction"]
tags:
  vertical: [coding, ops]
  category: tooling
  core: false
---

# Multi-Modal Intake Processing Skill

> **You are processing a non-text file that needs content extraction.**
> This skill tells you when, how, and why to use the intake system.

---

## 1. When to Load This Skill

Load this skill when you encounter any of these trigger conditions:

| Trigger | Example |
|---------|---------|
| **File type is non-plaintext** | PDF, image (PNG/JPG/GIF/WebP), audio (MP3/WAV), video (MP4), diagram |
| **User asks to "read" a file** | "Read this PDF", "What's in this image?" |
| **User asks to "extract"** | "Extract text from this diagram", "Extract data from this scan" |
| **User asks to "process"** | "Process this document", "Process this receipt" |
| **User asks to "analyze"** | "Analyze this chart", "Analyze this screenshot" |
| **User asks to "summarize"** | "Summarize this PDF", "Summarize this audio" |
| **File path in conversation** | User references a path to a non-text file that needs content extraction |
| **LucidChart JSON** | Any `.json` file from LucidChart that needs structured extraction |

**If the file is plain text** (`.txt`, `.md`, `.py`, `.js`, `.json`, `.yaml`, `.toml`, etc.), use standard file reading — do NOT use the intake tool.

---

## 2. The Three Intake Surfaces

Axiom provides three ways to process multi-modal files:

| Surface | Command | When to Use |
|---------|---------|-------------|
| **CLI** | `axiom intake <file>` | Humans running directly in terminal |
| **OpenCode Tool** | `intake_process_file` | **Agents in OpenCode sessions — USE THIS ONE** |
| **HTTP API** | `POST /api/intake/process` | External clients, programmatic access |

> **For agents: Always use the OpenCode tool `intake_process_file`.** This is the agent-native surface.

---

## 3. How to Use the OpenCode Tool

### Tool: `intake_process_file`

```text
Parameters:
  file_path: string      # Path to the file (relative to workspace root or absolute within workspace)
  prompt: string?        # (optional) Custom extraction prompt
  processor: string?     # (optional) Override processor — default is model_direct (Bedrock)
  dry_run: boolean?      # (optional) Validate without calling API
```

### Example Calls

```python
# Extract text from a PDF
intake_process_file(file_path="path/to/document.pdf")

# Analyze an image with custom prompt
intake_process_file(
    file_path="path/to/screenshot.png",
    prompt="Extract all UI elements, text, and layout information from this screenshot"
)

# Transcribe audio with specific processor
intake_process_file(
    file_path="path/to/interview.mp3",
    processor="whisper_local"
)

# Extract structured data from LucidChart diagram
intake_process_file(
    file_path="path/to/diagram.json",
    prompt="Extract all nodes, edges, and labels from this LucidChart diagram as a structured list."
)

# Validate without calling API
intake_process_file(
    file_path="path/to/document.pdf",
    dry_run=True
)
```

---

## 4. Processor Selection Guide

| File Type | Recommended Processor | Notes |
|-----------|----------------------|-------|
| **PDF** | `model_direct` | Bedrock Converse API direct bypass — best for text extraction |
| **Image (PNG/JPG/GIF/WebP)** | `model_direct` | Bedrock vision models — extracts text, describes visuals |
| **Audio (MP3/WAV)** | `whisper_local` | Local Whisper for transcription; falls back to AWS Transcribe |
| **Video** | `keyframes` | Extract keyframes first, then process each as image |
| **LucidChart JSON** | `model_direct` | Structured data extraction from diagram JSON |
| **Large PDF (>4.5 MB)** | `docling` | Self-hosted Docling (when available) — better for huge documents |

### Processor Allowlist

Only these processors are allowed. Unknown processors will be rejected:

- `model_direct` — Bedrock Converse API (default, works for PDF + images)
- `ollama` — Local Ollama LLM server (no AWS credentials needed)
- `whisper_local` — Local Whisper transcription (P4+)
- `transcribe` — AWS Transcribe (P4+)
- `keyframes` — Video keyframe extraction (P5+)
- `docling` — Self-hosted Docling for large PDFs (P3+)
- `unstructured` — Self-hosted Unstructured.io (P3+)
- `textract` — AWS Textract OCR (P3+)
- `pages_as_images` — Convert PDF pages to images, then process (P3+)
- `nova_sonic` — AWS Nova Sonic audio model (P4+)
- `omniparse` — Self-hosted OmniParse (P5+)
- `tesseract` — Local Tesseract OCR (P3+)

Processors marked P3+ require self-hosted service deployment or additional AWS services.
For P1/P2 work, use `model_direct` (Bedrock) or `ollama` (local).

---

## 5. Security Rules (Always Apply)

### Workspace Boundary Enforcement
- File paths are validated against the workspace boundary
- **Never pass absolute paths outside the repo**
- If the user provides a path outside the workspace, ask for a path within the workspace

### Credential Chain
- Credentials come from the AWS credential chain
- **Never pass API keys as parameters**
- Ensure AWS credentials are configured (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, or ~/.aws/credentials)

### Processor Validation
- The `--processor` flag is validated against an allowlist
- Unknown processors are rejected with `unknown_processor` error

---

## 6. Error Handling

| Error | Cause | Fix |
|-------|-------|-----|
| `path_outside_workspace` | File path is outside the repo | Ask user for a path within the workspace |
| `unknown_processor` | Invalid processor name | Use one from the allowlist (Section 4) |
| `RuntimeError: No AWS credentials` | AWS credentials not configured | User needs to set AWS_ACCESS_KEY_ID or configure ~/.aws/credentials |
| `FileNotFoundError` | File doesn't exist | Verify the path is correct relative to the workspace root |
| `Unsupported file type` | File type not supported | Use a supported file type from Section 4 |

---

## 7. Usage Patterns

### Pattern 1: User says "analyze this PDF"
```
User: "Can you analyze this PDF and tell me what's in it?"

Your action:
  1. Load this skill (intake-processing)
  2. Call: intake_process_file(file_path="path/to/file.pdf")
  3. Present the extracted content to the user
```

### Pattern 2: User says "what's in this image"
```
User: "What's in this screenshot?"

Your action:
  1. Load this skill (intake-processing)
  2. Call: intake_process_file(file_path="path/to/image.png")
  3. Describe the visual content to the user
```

### Pattern 3: User says "transcribe this audio"
```
User: "Transcribe this interview audio"

Your action:
  1. Load this skill (intake-processing)
  2. Call: intake_process_file(file_path="path/to/audio.mp3", processor="whisper_local")
  3. Present the transcription to the user
```

### Pattern 4: User says "extract from diagram"
```
User: "Extract all the nodes and edges from this LucidChart diagram"

Your action:
  1. Load this skill (intake-processing)
  2. Call: intake_process_file(
        file_path="path/to/diagram.json",
        prompt="Extract all nodes, edges, and labels from this LucidChart diagram as a structured list."
      )
  3. Present the structured extraction to the user
```

---

## 8. Related Specs

| Topic | Spec |
|-------|------|
| Multi-Modal Intake | `specs/82-Multi-Modal-Intake-And-Document-Processing.md` |
| OpenCode Tools | `specs/13-Command-Registry.md` |
| Workspace Boundary | `AGENTS.md` (Workspace Boundary section) |

---

## 9. Quick Reference Card

```text
WHEN to use:
  - Non-text files (PDF, image, audio, video, diagram)
  - User asks to read/extract/process/analyze/summarize a file

HOW to use:
  intake_process_file(
    file_path="path/to/file",
    prompt="optional custom prompt",
    processor="optional override",
    dry_run=True  # validate only
  )

PROCESSOR selection:
  - PDF/Image/LucidChart → model_direct
  - Audio → whisper_local
  - Video → keyframes (extract frames first)
  - Large PDF → docling (when available)

SECURITY:
  - Paths must be within workspace
  - Credentials from AWS chain (never pass keys)
  - Processors validated against allowlist
```

---

(End of file)