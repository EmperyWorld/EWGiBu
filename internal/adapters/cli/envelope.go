package cli

import (
	"encoding/json"
	"io"
)

// SchemaVersion is the machine-output envelope contract version (TZ.md §12.4).
const SchemaVersion = "iacpack.cli/v1"

// Envelope is the common JSON document for every --output json command.
type Envelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	OK            bool   `json:"ok"`
	Result        any    `json:"result"`
	Diagnostics   []any  `json:"diagnostics"`
}

// WriteJSON writes exactly one JSON document with a trailing newline.
func WriteJSON(w io.Writer, command string, ok bool, result any, diagnostics []any) error {
	if diagnostics == nil {
		diagnostics = []any{}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(Envelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		OK:            ok,
		Result:        result,
		Diagnostics:   diagnostics,
	})
}
