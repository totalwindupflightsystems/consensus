// Package cli implements the CLI command definitions (SPEC-016).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/016-cli-interface.md plan=phase-4/task-4-1/step-4-1-1 impl=internal/cli/formatter.go
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// Format is the output format for CLI results.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// Formatter writes structured output in the configured format.
type Formatter struct {
	w         io.Writer
	format    Format
	quiet     bool
	emptyHint string // optional hint printed when a table is empty (replaces "(no results)")
}

// NewFormatter creates a formatter for the given output writer and format.
func NewFormatter(w io.Writer, format Format, quiet bool) *Formatter {
	if w == nil {
		w = os.Stdout
	}
	return &Formatter{w: w, format: format, quiet: quiet}
}

// SetWriter changes the output writer (useful for testing).
func (f *Formatter) SetWriter(w io.Writer) {
	f.w = w
}

// SetEmptyHint configures a custom message printed when PrintTable encounters
// an empty result set. When set, this replaces the default "(no results)".
// Pass an empty string to restore the default.
func (f *Formatter) SetEmptyHint(hint string) {
	f.emptyHint = hint
}

// Print writes a single value in the configured format.
// For table format, this falls back to JSON if the value is not []map or []struct.
func (f *Formatter) Print(v any) error {
	if f.quiet {
		return nil
	}

	switch f.format {
	case FormatJSON:
		return f.printJSON(v)
	case FormatYAML:
		return f.printYAML(v)
	default:
		return f.printTable(v)
	}
}

// PrintTable writes tabular data with column headers.
func (f *Formatter) PrintTable(v any, headers []string) error {
	if f.quiet {
		return nil
	}

	switch f.format {
	case FormatJSON:
		return f.printJSON(v)
	case FormatYAML:
		return f.printYAML(v)
	default:
		return f.printTableWithHeaders(v, headers)
	}
}

// PrintText writes a plain text message (honors --quiet).
func (f *Formatter) PrintText(format string, args ...any) {
	if f.quiet {
		return
	}
	fmt.Fprintf(f.w, format, args...)
}

// Println writes a line (honors --quiet).
func (f *Formatter) Println(args ...any) {
	if f.quiet {
		return
	}
	fmt.Fprintln(f.w, args...)
}

// printJSON marshals the value as indented JSON.
func (f *Formatter) printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f.w, string(data))
	return err
}

// printYAML marshals the value as YAML.
func (f *Formatter) printYAML(v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f.w, string(data))
	return err
}

// printTable formats the value as a tab-separated table.
func (f *Formatter) printTable(v any) error {
	return f.printJSON(v) // fallback for single values
}

// printTableWithHeaders formats a slice of maps as aligned columns.
func (f *Formatter) printTableWithHeaders(v any, headers []string) error {
	// Marshal to JSON, unmarshal to []map to get generic table data.
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return f.printJSON(v) // fallback
	}

	if len(rows) == 0 {
		if f.emptyHint != "" {
			fmt.Fprintln(f.w, f.emptyHint)
		} else {
			fmt.Fprintln(f.w, "(no results)")
		}
		return nil
	}

	tw := tabwriter.NewWriter(f.w, 2, 4, 2, ' ', 0)

	// Print headers
	for _, h := range headers {
		fmt.Fprintf(tw, "%s\t", strings.ToUpper(h))
	}
	fmt.Fprintln(tw)

	// Print rows
	for _, row := range rows {
		for _, h := range headers {
			val := row[h]
			if val == nil {
				val = ""
			}
			s := fmt.Sprintf("%v", val)
			// Truncate long values for table display
			if len(s) > 60 {
				s = s[:57] + "..."
			}
			fmt.Fprintf(tw, "%s\t", s)
		}
		fmt.Fprintln(tw)
	}

	return tw.Flush()
}
