---
name: image-generation-bedrock-axiom
description: Generate images via AWS Bedrock (Titan v2) with prompt stacking, style presets, and reproducible metadata sidecars. Load when generating hero images, illustrations, icons, textures, or any visual assets for frontend work.
tags:
  vertical: [coding]
  category: development
  core: false
---

# AI Image Generation (Bedrock)

## Overview & Purpose

This skill teaches agents how to generate images using AWS Bedrock's Titan Image Generator v2 model. Use it when:

- Building frontend components that need visual assets (hero backgrounds, illustrations, icons)
- Creating marketing assets, documentation diagrams, or placeholder images
- Generating responsive image sets for web delivery
- Adapting existing images to match project style

**Key innovation**: Prompt stacking — build rich prompts by layering project style guide + user intent + aesthetic direction + constraints.

## Prerequisites

**AWS Credentials Required**

The CLI uses boto3's standard credential chain (env vars → `~/.aws/credentials` → IAM role). Without valid credentials, you will see:

```
Error: AWS credentials not found. Configure via `aws configure` or set AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY environment variables.
```

**Bedrock Model Access Required**

Bedrock image models require explicit opt-in per region:

1. Navigate to AWS Bedrock console → Model access
2. Request access for "Titan Image Generator v2"
3. Wait for approval (typically instant for Amazon models)

Without model access enabled, you will see:

```
Error: Bedrock model access denied. Enable 'amazon.titan-image-generator-v2:0' in the AWS Bedrock console.
```

**Regional Availability**: Titan v2 available in us-east-1, us-west-2, eu-west-1, ap-northeast-1.

## Prompt Engineering (5-Layer Stack)

Build prompts bottom-up. The style guide is the foundation — read it first.

```
Layer 5: Technical Parameters    ← resolution, quality, seed, cfg scale
Layer 4: Negative Constraints     ← what to avoid (text, watermarks, etc.)
Layer 3: Style & Aesthetic        ← art style, mood, lighting, composition
Layer 2: User Intent              ← "a hero image for our dashboard"
Layer 1: Project Style Guide      ← colors, theme, typography, brand mood
```

**Layer 1 — Style Guide (MUST attempt to read)**

Sources (check in order):
1. Design system file: `.axiom/design-system.md`, `design-tokens.json`
2. CSS variables: scan for `--color-*`, `--font-*` in CSS/SCSS
3. Tailwind/theme config: `tailwind.config.js`, `theme.ts`
4. Existing UI code: what colors/fonts are in use?
5. README: domain context (fintech? devtools? healthcare?)

Extract: color palette (with hex), theme (dark/light), typography mood, brand mood, domain.

**Layer 2 — User Intent**

The specific request: what image is needed and where it will be used.

**Layer 3 — Style Preset**

Select from preset library (see below) based on Layers 1 and 2.

**Layer 4 — Negative Constraints**

Default: `"Avoid: text, watermarks, logos, signatures, blurry, low quality, distorted, deformed"`

**Layer 5 — Technical Parameters**

- `width`/`height`: 320-1408 (multiples of 16), default 1024×1024
- `quality`: `"standard"` or `"premium"`
- `cfgScale`: 1.0-10.0 (how closely to follow prompt), default 8.0
- `seed`: 0-2147483647 (for reproducibility)

**Prompt Length**: Titan v2 has 512-char limit. If assembled prompt exceeds, truncate in priority order: Layer 4 → Layer 3 → Layer 1 → NEVER truncate Layer 2 (user intent).

## Style Presets (≥8)

| Preset | Prompt Fragment | When to Use |
|--------|----------------|-------------|
| `photorealistic` | `"photorealistic, 8k, sharp focus, natural lighting"` | Hero photos, marketing imagery |
| `oil-painting` | `"oil painting, impressionist, textured brushstrokes, warm palette"` | Artistic backgrounds, editorial |
| `watercolor` | `"watercolor illustration, soft edges, translucent washes"` | Soft, artistic illustrations |
| `minimalist` | `"minimalist, clean lines, flat design, limited color palette"` | Icons, simple graphics |
| `cinematic` | `"cinematic, dramatic lighting, film grain, wide angle"` | Hero backgrounds, dramatic scenes |
| `anime` | `"anime style, vibrant colors, cel shading, detailed linework"` | Character art, stylized illustrations |
| `sketch` | `"pencil sketch, hand-drawn, cross-hatching, monochrome"` | Wireframes, concept art |
| `isometric` | `"isometric 3D, clean geometry, soft shadows, pastel colors"` | Diagrams, technical illustrations |

**Example assembly**:

```
Layer 1: "dark theme, navy background (#0A1628), electric blue accent (#00D4FF), developer tools product"
Layer 2: "a hero image for the fintech dashboard"
Layer 3: "abstract geometric composition, clean lines, gradient depth, modern digital art"
Layer 4: "Avoid: text, watermarks, logos, blurry, low quality"

Assembled: "dark theme, navy background (#0A1628), electric blue accent (#00D4FF), developer tools product. A hero image for the fintech dashboard. Abstract geometric composition, clean lines, gradient depth, modern digital art. Avoid: text, watermarks, logos, blurry, low quality"
```

## CLI Command Reference

### `axiom image generate`

Generate a new image from text prompt.

```bash
# Basic generation (requires AWS credentials)
axiom image generate --prompt "a sunset over mountains" --output out.png  # Requires: AWS credentials + Bedrock model access

# With dimensions
axiom image generate --prompt "a sunset" --output out.png --width 512 --height 512  # Requires: AWS credentials + Bedrock model access

# With JSON output (matches API schema)
axiom image generate --prompt "a sunset" --output out.png --json  # Requires: AWS credentials + Bedrock model access
```

**Key flags**:
- `--prompt` (required): Assembled prompt text
- `--output` / `-o` (required): Output file path
- `--width` / `--height`: Dimensions (default 1024×1024)
- `--negative`: Negative prompt
- `--model`: Model ID (default: `amazon.titan-image-generator-v2:0`)
- `--quality`: `"standard"` or `"premium"`
- `--cfg-scale`: Prompt adherence 1.0-10.0 (default 8.0)
- `--seed`: Reproducibility seed
- `--json`: Machine-readable output

### `axiom image resize`

Resize an image (local operation, no AWS needed).

```bash
# Single resize
axiom image resize --input in.png --output out.png --width 512 --height 512

# Responsive set (lg/md/sm)
axiom image resize --input hero.png --responsive --output-dir assets/images/
```

### `axiom image info`

Show image metadata (local operation, no AWS needed).

```bash
# Human-readable
axiom image info --input out.png

# JSON output
axiom image info --input out.png --json
```

## Model Selection Guidance

**v1**: Only `amazon.titan-image-generator-v2:0` is supported.

| Model | Strengths | v1 Status |
|-------|-----------|-----------|
| Titan Image Generator v2 | Reliable, good quality, supports image-to-image | **Supported** |
| Nova Canvas v1 | Advanced editing, color-guided generation | **[DEFERRED]** |
| Titan Image Generator v1 | Older, still available | **[DEFERRED]** |

Use Titan v2 for all v1 generation. Nova Canvas ships after v1 proves the core workflow.

## Error Handling Guidance

| Error | Cause | Resolution |
|-------|-------|------------|
| `credentials_missing` | AWS credentials not configured | Run `aws configure` or set `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` env vars |
| `invalid_dimensions` | Width/height not multiples of 16 or out of range (320-1408) | Adjust dimensions to multiples of 16 within 320-1408 |
| `invalid_model` | Model ID not recognized | Use `amazon.titan-image-generator-v2:0` for v1 |
| `rate_limited` | Too many requests to Bedrock | Retry with exponential backoff (max 3 retries) |
| `generation_failed` | Bedrock API error | Check AWS service status, verify region availability |
| `model_access_denied` | Bedrock model access not enabled | Enable model in AWS Bedrock console → Model access |
| `content_filtered` | Prompt rejected by content filter | Modify prompt to comply with AWS Acceptable Use Policy |

## Output Conventions

**File locations**:

| Use Case | Output Path |
|----------|-------------|
| Project asset | `assets/images/<descriptive-name>.png` |
| Component-specific | `src/components/<component>/assets/<name>.png` |
| Documentation | `docs/images/<name>.png` |
| Temporary/preview | `/tmp/axiom-image-gen/<name>.png` |

**Sidecar metadata** (`.meta.json`):

Every generated image gets a sidecar file with the full recreation recipe:

```json
{
  "schema_version": 1,
  "generated_at": "2026-04-03T...",
  "command": "axiom image generate ...",
  "image": {
    "path": "assets/images/hero-bg.png",
    "sha256": "a3f2b8c1...",
    "format": "png",
    "width": 1024,
    "height": 1024,
    "size_bytes": 867328
  },
  "generation": {
    "model": "amazon.titan-image-generator-v2:0",
    "provider": "bedrock",
    "seed": 42,
    "task_type": "TEXT_IMAGE"
  },
  "prompt_assembled": "dark theme, navy background...",
  "prompt_layers": {
    "intent": "a hero image for the fintech dashboard"
  }
}
```

**Agent responsibilities**:
- Create output directory if needed
- Use descriptive filenames (not `image-1.png`)
- Report file path and size to user
- Add large images (>500KB) to `.gitignore` and suggest Git LFS

## Cost Awareness

**Titan v2 pricing** (approximate, verify current AWS pricing):

- Standard quality: ~$0.008 per image (512×512), ~$0.01 per image (1024×1024)
- Premium quality: ~$0.012 per image (1024×1024)

**Cost controls**:
- The API response includes `estimated_cost_usd` field
- Warn before generating >5 images in a single session
- No batch generation without explicit user approval

**Best practices**:
- Generate at highest reasonable resolution, then resize down
- Use `axiom image resize --responsive` for web delivery
- Convert to WebP for 25-35% size reduction when transparency not needed