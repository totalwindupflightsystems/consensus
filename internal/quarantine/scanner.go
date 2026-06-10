// Package quarantine implements the Cognitive Firewall (SPEC-005 §Cognitive Firewall)
// for scanning untrusted external data before it enters agent memory.
//
// The scanner uses regex + heuristic-based pattern matching for MVP, detecting
// SQL injection, XSS, prompt injection, and other attack patterns in payloads.
// The architecture is designed to be replaceable with a fast local model later.
//
// axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/013-webhooks-and-events.md plan=phase-1/task-1/step-1
package quarantine

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// ScanResult — the output of scanning a quarantined event
// ============================================================================

// ScanResult is the outcome of scanning external data through the cognitive firewall.
type ScanResult struct {
	Status         string   `json:"status"`          // "approved" or "rejected"
	ConfidenceScore float64 `json:"confidence_score"` // 0.0 (clean) to 1.0 (certain malicious)
	Reason         string   `json:"reason"`           // Human-readable explanation (empty if approved)
	ScannerVersion string   `json:"scanner_version"`  // Version of the scanner that produced this result
	MatchedRules   []string `json:"matched_rules,omitempty"` // Which rules triggered (rejected only)
}

const (
	// ScanApproved means the content passed all security checks.
	ScanApproved = "approved"

	// ScanRejected means the content was flagged as potentially malicious.
	ScanRejected = "rejected"

	// ScannerVersion identifies this scanner implementation.
	ScannerVersion = "axiom-quarantine-v1.0.0-regex"

	// DefaultConfidenceThreshold is the minimum confidence score to reject content.
	DefaultConfidenceThreshold = 0.3
)

// ============================================================================
// Scan Rule Definitions
// ============================================================================

// scanRule is a single heuristic pattern with a weight for confidence scoring.
type scanRule struct {
	Name        string
	Pattern     *regexp.Regexp
	Weight      float64 // contribution to confidence score (0.0-1.0)
	Description string
}

// scanRuleGroup is a category of related scan rules.
type scanRuleGroup struct {
	Name  string
	Rules []scanRule
}

// precompiledScanRules are the heuristic patterns used by the scanner.
// Each pattern contributes to the confidence score if matched.
var precompiledScanRules = []scanRuleGroup{
	{
		Name: "sql_injection",
		Rules: []scanRule{
			{
				Name:        "SQLI-UNION",
				Pattern:     regexp.MustCompile(`(?i)\bUNION\s+(ALL\s+)?SELECT\b`),
				Weight:      0.6,
				Description: "UNION SELECT statement in payload",
			},
			{
				Name:        "SQLI-OR-1=1",
				Pattern:     regexp.MustCompile(`(?i)\bOR\s+['"]?\d+['"]?\s*=\s*['"]?\d+['"]?\b`),
				Weight:      0.5,
				Description: "OR comparison always true (OR 1=1 pattern)",
			},
			{
				Name:        "SQLI-DROP-TABLE",
				Pattern:     regexp.MustCompile(`(?i)\bDROP\s+TABLE\b`),
				Weight:      0.8,
				Description: "DROP TABLE statement in content",
			},
			{
				Name:        "SQLI-DELETE-FROM",
				Pattern:     regexp.MustCompile(`(?i)\bDELETE\s+FROM\b`),
				Weight:      0.7,
				Description: "DELETE FROM statement in content",
			},
			{
				Name:        "SQLI-EXEC",
				Pattern:     regexp.MustCompile(`(?i)\bEXEC\b.*\bXP_`),
				Weight:      0.7,
				Description: "SQL stored procedure execution (xp_)",
			},
			{
				Name:        "SQLI-INTO-OUTFILE",
				Pattern:     regexp.MustCompile(`(?i)\bINTO\s+OUTFILE\b`),
				Weight:      0.8,
				Description: "INTO OUTFILE writes file to filesystem",
			},
			{
				Name:        "SQLI-SLEEP-BENCHMARK",
				Pattern:     regexp.MustCompile(`(?i)\bSLEEP\s*\(\s*\d+\s*\)|\bBENCHMARK\s*\(`),
				Weight:      0.6,
				Description: "Time-based blind SQL injection (SLEEP/BENCHMARK)",
			},
		},
	},
	{
		Name: "xss",
		Rules: []scanRule{
			{
				Name:        "XSS-SCRIPT-TAG",
				Pattern:     regexp.MustCompile(`(?i)<\s*script[^>]*>`),
				Weight:      0.7,
				Description: "Script tag in content",
			},
			{
				Name:        "XSS-JAVASCRIPT-PROTOCOL",
				Pattern:     regexp.MustCompile(`(?i)\bjavascript\s*:\s*\S+`),
				Weight:      0.5,
				Description: "javascript: protocol handler",
			},
			{
				Name:        "XSS-ONERROR",
				Pattern:     regexp.MustCompile(`(?i)\bonerror\s*=`),
				Weight:      0.5,
				Description: "HTML onerror event handler",
			},
			{
				Name:        "XSS-ONLOAD",
				Pattern:     regexp.MustCompile(`(?i)\bonload\s*=`),
				Weight:      0.4,
				Description: "HTML onload event handler",
			},
			{
				Name:        "XSS-DOCUMENT-COOKIE",
				Pattern:     regexp.MustCompile(`(?i)\bdocument\.cookie\b`),
				Weight:      0.6,
				Description: "document.cookie access (XSS data exfiltration)",
			},
			{
				Name:        "XSS-EVAL",
				Pattern:     regexp.MustCompile(`(?i)\beval\s*\(`),
				Weight:      0.5,
				Description: "JavaScript eval() call",
			},
			{
				Name:        "XSS-FROM-CHARCODE",
				Pattern:     regexp.MustCompile(`(?i)\bString\.fromCharCode\b`),
				Weight:      0.5,
				Description: "String.fromCharCode (obfuscated XSS)",
			},
		},
	},
	{
		Name: "prompt_injection",
		Rules: []scanRule{
			{
				Name:        "PI-IGNORE-INSTRUCTIONS",
				Pattern:     regexp.MustCompile(`(?i)(ignore|disregard|forget)\s+(all\s+)?(previous|prior|above)\s+(instructions|directives|commands|rules|prompts)`),
				Weight:      0.8,
				Description: "Prompt injection: ignore previous instructions",
			},
			{
				Name:        "PI-SYSTEM-OVERRIDE",
				Pattern:     regexp.MustCompile(`(?i)^\s*(system|admin|root)\s*:\s*(say|override|command|instruction)`),
				Weight:      0.7,
				Description: "Prompt injection: system override attempt",
			},
			{
				Name:        "PI-YOU-ARE-NOW",
				Pattern:     regexp.MustCompile(`(?i)you\s+are\s+now\s+(a\s+|an\s+)?(free|unleashed|unrestricted|DAN|jailbreak)`),
				Weight:      0.9,
				Description: "Prompt injection: jailbreak/DAN persona",
			},
			{
				Name:        "PI-OUTPUT-FORMAT",
				Pattern:     regexp.MustCompile(`(?i)(output|respond|reply)\s+(only|just|exclusively)\s+(in|with)\s+(json|raw|unfiltered)`),
				Weight:      0.5,
				Description: "Prompt injection: output format override",
			},
			{
				Name:        "PI-SECURITY-BYPASS",
				Pattern:     regexp.MustCompile(`(?i)(bypass|circumvent|disable)\s+(security|restrictions|guardrails|safeguards|content.?policy)`),
				Weight:      0.7,
				Description: "Prompt injection: security bypass attempt",
			},
			{
				Name:        "PI-REVEAL-PROMPT",
				Pattern:     regexp.MustCompile(`(?i)(reveal|show|print|leak|disclose)\s+(your\s+)?(prompt|system\s+prompt|instructions|system\s+message)`),
				Weight:      0.6,
				Description: "Prompt injection: prompt extraction attempt",
			},
		},
	},
	{
		Name: "suspicious_structure",
		Rules: []scanRule{
			{
				Name:        "SUS-NULL-BYTES",
				Pattern:     regexp.MustCompile(`\x00`),
				Weight:      0.6,
				Description: "Null bytes in content (binary smuggling)",
			},
			{
				Name:        "SUS-BASE64-EXEC",
				Pattern:     regexp.MustCompile(`(?i)[A-Za-z0-9+/]{100,}={0,2}\s*\.\s*(exec|run|eval|call)`),
				Weight:      0.4,
				Description: "Large base64 blob followed by execution",
			},
			{
				Name:        "SUS-NESTED-JSON",
				Pattern:     regexp.MustCompile(`\{[^}]*\{[^}]*\{[^}]*\{`),
				Weight:      0.3,
				Description: "Deeply nested JSON (4+ levels)",
			},
		},
	},
}

// ============================================================================
// Scanner Configuration
// ============================================================================

// ScannerConfig allows tuning the scanner behavior.
type ScannerConfig struct {
	// ConfidenceThreshold is the minimum score to reject content (0.0-1.0).
	// Default: DefaultConfidenceThreshold (0.3)
	ConfidenceThreshold float64

	// EnabledGroups allows selectively disabling rule groups by name.
	// If empty, all groups are enabled.
	EnabledGroups []string
}

// DefaultScannerConfig returns the default scanner configuration.
func DefaultScannerConfig() ScannerConfig {
	return ScannerConfig{
		ConfidenceThreshold: DefaultConfidenceThreshold,
		EnabledGroups:       []string{}, // all groups enabled
	}
}

// ============================================================================
// Core Scanning Function
// ============================================================================

// ScanQuarantinedEvent scans content for malicious patterns and returns a ScanResult.
//
// The function:
//  1. Checks the content against all enabled heuristic patterns
//  2. Calculates a weighted confidence score from matched patterns
//  3. Returns approved (score < threshold) or rejected (score >= threshold) with details
//
// This is an MVP implementation using regex. The architecture supports replacing
// the regex engine with a fast local model (e.g., Llama 3 8B) in the future
// while keeping the same ScanResult contract.
func ScanQuarantinedEvent(content string, config ScannerConfig) ScanResult {
	if config.ConfidenceThreshold <= 0 {
		config.ConfidenceThreshold = DefaultConfidenceThreshold
	}

	var score float64
	var matchedRules []string
	var matchedDescriptions []string

	// Build group allow-set
	allGroups := len(config.EnabledGroups) == 0
	groupAllowed := make(map[string]bool, len(config.EnabledGroups))
	for _, g := range config.EnabledGroups {
		groupAllowed[g] = true
	}

	// Check each rule group
	for _, group := range precompiledScanRules {
		if !allGroups && !groupAllowed[group.Name] {
			continue
		}
		for _, rule := range group.Rules {
			if rule.Pattern.MatchString(content) {
				score += rule.Weight
				matchedRules = append(matchedRules, rule.Name)
				matchedDescriptions = append(matchedDescriptions, fmt.Sprintf("[%s] %s", rule.Name, rule.Description))
			}
		}
	}

	// Clamp score to [0.0, 1.0]
	if score > 1.0 {
		score = 1.0
	}

	// Build result
	result := ScanResult{
		Status:          ScanApproved,
		ConfidenceScore: score,
		ScannerVersion:  ScannerVersion,
	}

	if score >= config.ConfidenceThreshold && len(matchedRules) > 0 {
		result.Status = ScanRejected
		result.Reason = fmt.Sprintf("Quarantine scanner flagged content: %s", strings.Join(matchedDescriptions, "; "))
		result.MatchedRules = matchedRules
	}

	return result
}

// ============================================================================
// Helper: Content Hash
// ============================================================================

// ContentHash returns an MD5 hex digest of the content.
// Used to populate external_quarantine.content_hash.
func ContentHash(content string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(content)))
}

// ============================================================================
// Helper: Source Type Mapping
// ============================================================================

// MapEventSourceToQuarantineType maps an event source (from external_events)
// to a quarantine source_type (for external_quarantine).
func MapEventSourceToQuarantineType(source string) string {
	switch source {
	case "webhook":
		return "api_response"
	case "email":
		return "api_response"
	case "cron":
		return "api_response"
	case "manual":
		return "user_paste"
	case "api":
		return "api_response"
	default:
		return "api_response"
	}
}
