---
description: Generate, resize, convert, or inspect images using the Axiom image service (Bedrock + local ops).
agent: dispatch-axiom
---

Generate or manipulate images using the Axiom image service. Supports text-to-image generation via AWS Bedrock, plus local resize, convert, and info operations.

Inputs:
- Action: $ACTION (required; one of: generate, resize, convert, info, models)
- Prompt: $PROMPT (required for generate; the text prompt for image generation)
- Input: $INPUT (required for resize/convert/info; path to input image)
- Output: $OUTPUT (optional; output path. Default: `_tmp/axiom-image-<timestamp>.<format>`)
- Model: $MODEL (optional; default: amazon.titan-image-generator-v2:0)
- Width: $WIDTH (optional; default: 1024)
- Height: $HEIGHT (optional; default: 1024)
- Format: $FORMAT (optional; for convert: png, jpeg, webp. Default: png)
- AWS Profile: $AWS_PROFILE (optional; AWS credentials profile for Bedrock)

Skills (load on demand):
- `image-generation-bedrock-axiom` — Full guide for prompt stacking, style presets, model capabilities, and sidecar metadata. Load when generating images.

Spec contract: `specs/72-AI-Image-Generation-Skill.md`

**Important**: All Python commands below run from the `.axiom/` directory (use `workdir` parameter set to `.axiom/`). The `PYTHONPATH=src` is required because the axiom package source lives at `.axiom/src/`.

Do:

**If $ACTION is "generate":**
1) Validate $PROMPT is not empty.
2) Load `image-generation-bedrock-axiom` skill for prompt stacking guidance.
3) Validate $MODEL is in the model registry (run: `PYTHONPATH=src python3 -c "from axiom.shared.image.models import MODEL_REGISTRY; print(list(MODEL_REGISTRY.keys()))"` from `.axiom/`).
4) Validate $WIDTH and $HEIGHT are valid for the model (Titan v2: min 320, step 16, max 4096).
5) **Check AWS credentials before generating** — if credentials are missing or invalid, return `status=blocked` with a clear message, NOT a Python traceback:
   ```bash
   PYTHONPATH=src AWS_PROFILE=${AWS_PROFILE:-default} python3 -c "
   import boto3, sys
   try:
       sts = boto3.client('sts')
       identity = sts.get_caller_identity()
       print(f'AWS OK: {identity[\"Arn\"]}')
   except Exception as e:
       print(f'AWS credentials not available: {e}', file=sys.stderr)
       sys.exit(1)
   "
   ```
   If this fails: return `status=blocked`, `summary="AWS credentials not available for Bedrock image generation. Configure via AWS_PROFILE or aws configure."`. Do NOT proceed to generation.
6) Generate the image:
   ```bash
   PYTHONPATH=src AWS_PROFILE=${AWS_PROFILE:-default} python3 -c "
   import asyncio
   from axiom.shared.image.service import ImageService
   async def gen():
       svc = ImageService()
       results = await svc.generate(
           prompt='$PROMPT',
           output_path='$OUTPUT',
           model='$MODEL',
           width=$WIDTH,
           height=$HEIGHT,
       )
       for r in results:
           print(f'Generated: {r.path} ({r.width}x{r.height}, {r.size_bytes} bytes)')
           print(f'Sidecar: {r.sidecar_path}')
           print(f'SHA256: {r.sha256}')
           print(f'Seed: {r.seed}')
   asyncio.run(gen())
   "
   ```
7) Report: image path, dimensions, file size, sidecar path, seed for reproducibility.

**If $ACTION is "resize":**
1) Validate $INPUT exists.
2) Resize using the image service:
   ```bash
   PYTHONPATH=src python3 -c "
   import asyncio
   from axiom.shared.image.service import ImageService
   async def resize():
       svc = ImageService()
       result = await svc.resize(
           input_path='$INPUT',
           output_path='$OUTPUT',
           width=$WIDTH,
           height=$HEIGHT,
       )
       print(f'Resized: {result.path} ({result.width}x{result.height}, {result.size_bytes} bytes)')
   asyncio.run(resize())
   "
   ```

**If $ACTION is "convert":**
1) Validate $INPUT exists and $FORMAT is valid (png, jpeg, webp).
2) Convert using the image service:
   ```bash
   PYTHONPATH=src python3 -c "
   import asyncio
   from axiom.shared.image.service import ImageService
   async def convert():
       svc = ImageService()
       result = await svc.resize(
           input_path='$INPUT',
           output_path='$OUTPUT',
           format='$FORMAT',
       )
       print(f'Converted: {result.path} ({result.width}x{result.height}, {result.size_bytes} bytes)')
   asyncio.run(convert())
   "
   ```

**If $ACTION is "info":**
1) Validate $INPUT exists.
2) Show image metadata:
   ```bash
   PYTHONPATH=src python3 -c "
   from pathlib import Path
   from PIL import Image
   p = Path('$INPUT')
   if not p.exists():
       print(f'Error: {p} not found')
       exit(1)
   img = Image.open(p)
   print(f'Path: {p}')
   print(f'Format: {img.format}')
   print(f'Dimensions: {img.width}x{img.height}')
   print(f'Mode: {img.mode}')
   print(f'Size: {p.stat().st_size} bytes')
   "
   ```

**If $ACTION is "models":**
1) List all available models from the registry:
   ```bash
   PYTHONPATH=src python3 -c "
   from axiom.shared.image.models import MODEL_REGISTRY
   for mid, info in MODEL_REGISTRY.items():
       status = '✅' if info.status == 'available' else '⏸️'
       print(f'{status} {mid}: {info.name} ({info.provider}) — {info.capabilities}')
   "
   ```

Error handling:
- AWS credential failures → `status=blocked` (not a traceback)
- Missing input file → `status=fail` with "File not found: <path>"
- Invalid model → `status=fail` with "Unknown model: <model_id>. Available: <list>"
- Invalid dimensions → `status=fail` with "Invalid dimensions: <w>x<h>. Constraints: <model constraints>"
- Bedrock API errors (throttling, access denied) → `status=fail` with the error message

Output:
- For generate: image path, dimensions, file size, sidecar metadata path, seed, estimated cost
- For resize/convert: output path, dimensions, file size
- For info: image metadata table
- For models: model list with status and capabilities
- **Always include `evidence.files_changed`** listing all output files created (image, sidecar, etc.)

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating which action ran and what was produced.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - For generate: image file path + sidecar metadata path
  - For resize/convert: output file path
  - For info/models: empty (read-only)
- `evidence.image_path`: full path to the primary output image (for generate/resize/convert)
- `evidence.sidecar_path`: full path to the sidecar metadata file (for generate)
- `evidence.action`: which action was performed (generate|resize|convert|info|models)
- `related_commands`: suggested follow-up commands
  - "To resize the generated image, run: `/axiom-image resize --input <path> --width <w> --height <h>`"
  - "To convert the image format, run: `/axiom-image convert --input <path> --format <fmt>`"
  - "To view image metadata, run: `/axiom-image info --input <path>`"

### Cross-References
- "Image generation skill is in: `.opencode/skills/image-generation-bedrock-axiom/SKILL.md`"
- "Spec: `specs/72-AI-Image-Generation-Skill.md`"
- "Output images are stored in: `_tmp/` (gitignored scratch space)"

axiom:trace work_item=image-gen-cli-core-01 spec=specs/72-AI-Image-Generation-Skill.md work_item=command-quality-01
