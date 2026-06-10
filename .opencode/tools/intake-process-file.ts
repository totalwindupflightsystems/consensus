import { tool } from "@opencode-ai/plugin"

import * as fs from "node:fs/promises"
import * as path from "node:path"
import { spawn } from "node:child_process"

const VALID_PROCESSORS = [
  "docling",
  "unstructured",
  "model_direct",
  "textract",
  "pages_as_images",
  "whisper_local",
  "transcribe",
  "nova_sonic",
  "keyframes",
  "omniparse",
  "tesseract",
] as const

type Processor = (typeof VALID_PROCESSORS)[number]

function isValidProcessor(p: string): p is Processor {
  return (VALID_PROCESSORS as readonly string[]).includes(p)
}

function mustBeUnderWorktree(worktree: string, target: string): string {
  const resolvedWorktree = path.resolve(worktree)
  const resolvedTarget = path.resolve(target)
  const rel = path.relative(resolvedWorktree, resolvedTarget)
  if (rel.startsWith(".." + path.sep) || rel === "..") {
    throw new Error(`Refusing to access outside worktree: ${target}`)
  }
  return resolvedTarget
}

function resolveFilePath(worktree: string, filePath: string): string {
  // If absolute, validate it's under worktree
  if (path.isAbsolute(filePath)) {
    return mustBeUnderWorktree(worktree, filePath)
  }
  // Relative path - resolve against worktree
  return mustBeUnderWorktree(worktree, path.join(worktree, filePath))
}

function buildCommand(
  filePath: string,
  prompt: string,
  processor: string,
  dryRun: boolean,
): string[] {
  // Use the unified 'intake process' command which routes by file type
  // (PDF → pdf_processor, image → image_processor, etc.)
  const args = ["intake", "process", filePath, "--json"]
  if (prompt) {
    args.push("--prompt", prompt)
  }
  if (processor) {
    args.push("--processor", processor)
  }
  if (dryRun) {
    args.push("--dry-run")
  }
  return args
}

function parseJsonOutput(output: string): unknown {
  // Handle potential leading/trailing whitespace and multiple JSON objects
  const trimmed = output.trim()
  // Find the first { or [ and parse from there
  const startIdx = trimmed.search(/[\[{]/)
  if (startIdx === -1) {
    throw new Error(`No JSON found in output: ${trimmed}`)
  }
  return JSON.parse(trimmed.slice(startIdx))
}

export default tool({
  description:
    "Use this tool when the user references a file (PDF, image, audio, diagram) that needs to be processed to extract text or content. This is the primary way to handle multi-modal files in Axiom. Returns extracted text/markdown content from the file, plus metadata (file hash, processor used, timestamp). File paths are validated against the workspace boundary. Credentials come from the AWS credential chain.",
  args: {
    file_path: tool.schema
      .string()
      .describe("Path to the file to process (relative to workspace root or absolute within workspace)"),
    prompt: tool.schema
      .string()
      .optional()
      .describe(
        "Custom extraction prompt. Default: 'Extract and return the full text content of this file in markdown format.'",
      ),
    processor: tool.schema
      .string()
      .optional()
      .describe(
        `Override processor. Default: 'model_direct'. Must be one of: ${VALID_PROCESSORS.join(", ")}`,
      ),
    dry_run: tool.schema
      .boolean()
      .optional()
      .describe("If true, validate inputs but don't call the API. Default: false"),
  },
  async execute(args, context) {
    // Validate processor if provided
    if (args.processor && !isValidProcessor(args.processor)) {
      return {
        success: false,
        error: `Invalid processor: ${args.processor}. Must be one of: ${VALID_PROCESSORS.join(", ")}`,
      }
    }

    // Resolve and validate file path
    const resolvedPath = resolveFilePath(context.worktree, args.file_path)

    // Check if file exists
    try {
      await fs.access(resolvedPath)
    } catch {
      return {
        success: false,
        error: `File not found: ${args.file_path}`,
        hint: "Ensure the path is relative to the workspace root or an absolute path within the workspace.",
      }
    }

    // Build the command
    const cmdArgs = buildCommand(
      args.file_path,
      args.prompt || "Extract and return the full text content of this file in markdown format.",
      args.processor || "model_direct",
      args.dry_run || false,
    )

    // For dry run, just return what would be called
    if (args.dry_run) {
      return {
        success: true,
        dry_run: true,
        command: `axiom ${cmdArgs.join(" ")}`,
        file_path: args.file_path,
        resolved_path: resolvedPath,
        processor: args.processor || "model_direct",
        prompt:
          args.prompt || "Extract and return the full text content of this file in markdown format.",
      }
    }

    // Execute the command
    return new Promise((resolve) => {
      const stdout: string[] = []
      const stderr: string[] = []

      const proc = spawn("axiom", cmdArgs, {
        cwd: context.worktree,
        env: { ...process.env },
        stdio: ["pipe", "pipe", "pipe"],
      })

      proc.stdout.on("data", (data) => {
        stdout.push(data.toString())
      })

      proc.stderr.on("data", (data) => {
        stderr.push(data.toString())
      })

      proc.on("close", (code) => {
        const stdoutStr = stdout.join("")
        const stderrStr = stderr.join("")

        if (code !== 0) {
          resolve({
            success: false,
            error: `axiom intake failed with exit code ${code}`,
            stderr: stderrStr,
            stdout: stdoutStr,
            command: `axiom ${cmdArgs.join(" ")}`,
          })
          return
        }

        try {
          const parsed = parseJsonOutput(stdoutStr)
          resolve({
            success: true,
            ...(parsed as object),
          })
        } catch (parseErr) {
          resolve({
            success: false,
            error: `Failed to parse JSON output: ${(parseErr as Error).message}`,
            raw_output: stdoutStr,
            stderr: stderrStr,
            command: `axiom ${cmdArgs.join(" ")}`,
          })
        }
      })

      proc.on("error", (err) => {
        resolve({
          success: false,
          error: `Failed to execute axiom: ${err.message}`,
          command: `axiom ${cmdArgs.join(" ")}`,
        })
      })
    })
  },
})