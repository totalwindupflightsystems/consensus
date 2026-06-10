// Package compression implements the vector-validated memory compression pipeline.
//
// This package provides the background compression worker that processes
// the compression_queue, generates summaries using model cascade (SPEC-002 §9),
// validates them via cosine similarity (SPEC-002 §8), and escalates through
// display tiers (raw → compressed → abstract → canonical) when quality gates fail.
//
// The compression loop:
//  1. Poll compression_queue for pending events
//  2. Generate embedding of original content
//  3. Select tier model and generate summary
//  4. Generate embedding of summary
//  5. Validate with cosine similarity (threshold: 0.85)
//  6. Accept, escalate, or fail
//
// axiom:trace work_item=vector-compression-01 spec=specs/002-memory.md,specs/011-canonical-definitions.md plan=phase-1/task-1-2 impl=internal/compression/compression.go
package compression

import (
	"fmt"
)

// ============================================================================
// Display Tier Ladder (SPEC-002 §8, SPEC-003 §6.2)
// ============================================================================

// DisplayTier represents the compression escalation level.
// Events progress through tiers as they are compressed:
//
//	Tier 0 (raw):       Original, full content — display_mode = 'full'
//	Tier 1 (compressed): First-level summary (40-60% compression)
//	Tier 2 (abstract):   Abstract-level summary (70-80% compression)
//	Tier 3 (canonical):  Canonical form (~90% compression, structured)
//
// The compression_queue.current_tier field tracks which tier an event
// currently occupies. Escalation happens when cosine similarity < threshold.
type DisplayTier int

const (
	// TierRaw is the original full content (display_mode = 'full').
	// No compression has been applied.
	TierRaw DisplayTier = iota // 0

	// TierCompressed is the first-level summary.
	// Display mode becomes 'compressed'. Target: 40-60% compression.
	TierCompressed // 1

	// TierAbstract is an abstract-level summary.
	// Higher compression ratio; used when Tier 1 fails quality gate.
	TierAbstract // 2

	// TierCanonical is the most compressed canonical form.
	// Structured format; used when all lower tiers fail quality gate.
	TierCanonical // 3
)

// String returns the human-readable tier name.
func (t DisplayTier) String() string {
	switch t {
	case TierRaw:
		return "raw"
	case TierCompressed:
		return "compressed"
	case TierAbstract:
		return "abstract"
	case TierCanonical:
		return "canonical"
	default:
		return fmt.Sprintf("tier(%d)", int(t))
	}
}

// DisplayMode returns the display_modes.mode value for this tier.
func (t DisplayTier) DisplayMode() string {
	switch t {
	case TierCompressed, TierAbstract, TierCanonical:
		return "compressed"
	default:
		return "full"
	}
}

// MaxTier is the highest compression tier.
const MaxTier = TierCanonical

// ============================================================================
// Default Thresholds
// ============================================================================

// DefaultCosineThreshold is the minimum cosine similarity score for accepting
// a compressed summary (SPEC-002 §8.2). Value: 0.85.
const DefaultCosineThreshold = 0.85

// ============================================================================
// Tier Escalation
// ============================================================================

// NextTier returns the next escalation tier after the given tier.
// Returns TierRaw for an invalid input, or the same tier if already at max.
func NextTier(current DisplayTier) DisplayTier {
	switch current {
	case TierRaw:
		return TierCompressed
	case TierCompressed:
		return TierAbstract
	case TierAbstract:
		return TierCanonical
	default:
		return TierCanonical // Already at max; stay here
	}
}

// ShouldEscalate returns true if the cosine similarity score is below threshold.
// When true, the compression should be retried with a more capable model.
// SPEC-002 §8.2, §9.
func ShouldEscalate(cosineScore float64, threshold float64) bool {
	return cosineScore < threshold
}

// CompressionResult indicates the outcome of a compression attempt.
type CompressionResult int

const (
	// ResultAccepted means the summary passed cosine similarity validation
	// and was written to memory_events.summary_text.
	ResultAccepted CompressionResult = iota

	// ResultRejectedEscalate means the summary failed validation and should
	// be retried with the next tier model.
	ResultRejectedEscalate

	// ResultFailed means the summary failed validation and all retry
	// attempts have been exhausted.
	ResultFailed
)

func (r CompressionResult) String() string {
	switch r {
	case ResultAccepted:
		return "accepted"
	case ResultRejectedEscalate:
		return "rejected_escalate"
	case ResultFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// TierFromInt converts an integer to a DisplayTier.
func TierFromInt(tier int) DisplayTier {
	switch tier {
	case 0:
		return TierRaw
	case 1:
		return TierCompressed
	case 2:
		return TierAbstract
	case 3:
		return TierCanonical
	default:
		if tier < 0 {
			return TierRaw
		}
		return TierCanonical
	}
}

// CompressionSummaryPrompt returns the system prompt for the LLM summarization,
// adapted to the target tier level.
func CompressionSummaryPrompt(tier DisplayTier) string {
	switch tier {
	case TierCompressed:
		return `You are a memory compression agent. Your task is to summarize the following agent conversation or memory event into a concise summary.

Requirements:
- Retain all key facts, decisions, and outcomes
- Preserve action items and unresolved questions
- Output only the summary text, no additional commentary
- Target length: 40-60% of the original
- Use plain text, not markdown`

	case TierAbstract:
		return `You are a memory compression agent performing abstract-level compression. Your task is to distill the following content into a high-level abstract.

Requirements:
- Capture only the essential meaning, decisions, and outcomes
- Omit implementation details and intermediate steps
- Target length: 20-30% of the original
- Output only the abstract text, no additional commentary
- Use plain text, not markdown`

	case TierCanonical:
		return `You are a memory compression agent performing canonical compression. Your task is to convert the following content into a canonical structured form.

Requirements:
- Extract: goal, key decisions, outcomes, action items, references
- Format each section concisely
- Target length: 10-15% of the original
- Output only the canonical form text, no additional commentary
- Use plain text, not markdown`

	default:
		return CompressionSummaryPrompt(TierCompressed)
	}
}

// CosineThresholdForTier returns the minimum cosine similarity threshold
// for the given tier. Higher tiers use a slightly relaxed threshold since
// more aggressive compression naturally diverges more from the original.
func CosineThresholdForTier(tier DisplayTier) float64 {
	switch tier {
	case TierCompressed:
		return 0.85
	case TierAbstract:
		return 0.80
	case TierCanonical:
		return 0.75
	default:
		return 0.85
	}
}
