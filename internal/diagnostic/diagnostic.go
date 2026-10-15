// Package diagnostic defines the stable, machine-readable diagnostic model.
package diagnostic

import (
	"sort"
	"strings"
)

// Severity ranks diagnostic importance for presentation and exit mapping.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Source locates a diagnostic in an authoring document.
type Source struct {
	File    string `json:"file,omitempty"`
	Pointer string `json:"pointer,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

// Diagnostic is one structured finding (TZ.md §12.3).
type Diagnostic struct {
	Code      string   `json:"code"`
	Severity  Severity `json:"severity"`
	Message   string   `json:"message"`
	Source    *Source  `json:"source,omitempty"`
	Hint      string   `json:"hint,omitempty"`
	Retryable bool     `json:"retryable"`
}

// Stable diagnostic codes used by the P01 walking slice.
const (
	CodeManifestParseError             = "MANIFEST.PARSE_ERROR"
	CodeManifestDuplicateKey           = "MANIFEST.DUPLICATE_KEY"
	CodeManifestAliasForbidden         = "MANIFEST.ALIAS_FORBIDDEN"
	CodeManifestAnchorForbidden        = "MANIFEST.ANCHOR_FORBIDDEN"
	CodeManifestMergeKeyForbidden      = "MANIFEST.MERGE_KEY_FORBIDDEN"
	CodeManifestMultipleDocuments      = "MANIFEST.MULTIPLE_DOCUMENTS"
	CodeManifestTooLarge               = "MANIFEST.TOO_LARGE"
	CodeManifestTooDeep                = "MANIFEST.TOO_DEEP"
	CodeManifestNonStringKey           = "MANIFEST.NON_STRING_KEY"
	CodeManifestUnsupportedAPI         = "MANIFEST.UNSUPPORTED_API_VERSION"
	CodeManifestUnsupportedKind        = "MANIFEST.UNSUPPORTED_KIND"
	CodeManifestUnsupportedProfile     = "MANIFEST.UNSUPPORTED_PROFILE"
	CodeManifestMutableImageTag        = "MANIFEST.MUTABLE_IMAGE_TAG"
	CodeManifestCustomTagForbidden     = "MANIFEST.CUSTOM_TAG_FORBIDDEN"
	CodeManifestPackageOptionConflict  = "MANIFEST.PACKAGE_OPTION_CONFLICT"
	CodeManifestPortConflict           = "MANIFEST.PORT_CONFLICT"
	CodeManifestInvalidDependency      = "MANIFEST.INVALID_DEPENDENCY"
	CodeManifestInvalidReference       = "MANIFEST.INVALID_REFERENCE"
	CodeManifestSecretMisuse           = "MANIFEST.SECRET_MISUSE"
	CodeManifestBuildNotAllowed        = "MANIFEST.BUILD_NOT_ALLOWED"
	CodeManifestInterpolationForbidden = "MANIFEST.INTERPOLATION_FORBIDDEN"

	CodeSchemaUnknownField      = "SCHEMA.UNKNOWN_FIELD"
	CodeSchemaMissingField      = "SCHEMA.MISSING_FIELD"
	CodeSchemaInvalidType       = "SCHEMA.INVALID_TYPE"
	CodeSchemaInvalidIdentifier = "SCHEMA.INVALID_IDENTIFIER"
	CodeSchemaInvalidSemver     = "SCHEMA.INVALID_SEMVER"
	CodeSchemaInvalidValue      = "SCHEMA.INVALID_VALUE"
	CodeSchemaInvalidDuration   = "SCHEMA.INVALID_DURATION"
	CodeSchemaInvalidPath       = "SCHEMA.INVALID_PATH"

	CodePolicyRegistryDenied   = "POLICY.REGISTRY_DENIED"
	CodePolicyCapabilityDenied = "POLICY.CAPABILITY_DENIED"
	CodePolicyUnsupportedField = "POLICY.UNSUPPORTED_FIELD"
	CodePolicyResourceCeiling  = "POLICY.RESOURCE_CEILING"

	CodeCLIInvalidUsage      = "CLI.INVALID_USAGE"
	CodeCLIUnsupportedSource = "CLI.UNSUPPORTED_SOURCE"
	CodeInputIOError         = "INPUT.IO_ERROR"
	CodeInternal             = "INTERNAL.INVARIANT"
	CodeBuildFrontend        = "BUILD.FRONTEND"

	CodeValidateSyntaxOnly = "VALIDATE.SYNTAX_ONLY"

	CodeImportParseError       = "IMPORT.PARSE_ERROR"
	CodeImportPathInvalid      = "IMPORT.PATH_INVALID"
	CodeImportUnsupportedField = "IMPORT.UNSUPPORTED_FIELD"
	CodeImportPolicyDenied     = "IMPORT.POLICY_DENIED"
	CodeImportInterpolation    = "IMPORT.INTERPOLATION"
	CodeImportEmpty            = "IMPORT.EMPTY"
	CodeImportSecretLike       = "IMPORT.SECRET_LIKE"
	CodeImportCollision        = "IMPORT.COLLISION"
	CodeImportProfileSkipped   = "IMPORT.PROFILE_SKIPPED"
	CodeImportMigrateGap       = "IMPORT.MIGRATE_GAP"
	CodeImportUserName         = "IMPORT.USER_NAME"
	CodeImportUserAbsent       = "IMPORT.USER_ABSENT"
	CodeImportEnvFrom          = "IMPORT.ENV_FROM"
)

// Error constructs an error-severity diagnostic.
func Error(code, message string) Diagnostic {
	return Diagnostic{
		Code:      code,
		Severity:  SeverityError,
		Message:   message,
		Retryable: false,
	}
}

// Warning constructs a warning-severity diagnostic.
func Warning(code, message string) Diagnostic {
	return Diagnostic{
		Code:      code,
		Severity:  SeverityWarning,
		Message:   message,
		Retryable: false,
	}
}

// Info constructs an info-severity diagnostic.
func Info(code, message string) Diagnostic {
	return Diagnostic{
		Code:      code,
		Severity:  SeverityInfo,
		Message:   message,
		Retryable: false,
	}
}

// WithSource attaches a source location.
func (d Diagnostic) WithSource(file, pointer string, line, column int) Diagnostic {
	d.Source = &Source{
		File:    file,
		Pointer: pointer,
		Line:    line,
		Column:  column,
	}
	return d
}

// WithHint attaches a remediation hint.
func (d Diagnostic) WithHint(hint string) Diagnostic {
	d.Hint = hint
	return d
}

// SortDeterministic orders diagnostics by file → pointer → code (TZ.md §12.3).
func SortDeterministic(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		af, bf := sourceFile(a), sourceFile(b)
		if af != bf {
			return af < bf
		}
		ap, bp := sourcePointer(a), sourcePointer(b)
		if ap != bp {
			return ap < bp
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
}

func sourceFile(d Diagnostic) string {
	if d.Source == nil {
		return ""
	}
	return d.Source.File
}

func sourcePointer(d Diagnostic) string {
	if d.Source == nil {
		return ""
	}
	return d.Source.Pointer
}

// HasError reports whether any diagnostic is severity error.
func HasError(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// ToMaps converts diagnostics to JSON-friendly maps for the CLI envelope.
func ToMaps(diags []Diagnostic) []any {
	out := make([]any, 0, len(diags))
	for _, d := range diags {
		m := map[string]any{
			"code":      d.Code,
			"severity":  string(d.Severity),
			"message":   d.Message,
			"retryable": d.Retryable,
		}
		if d.Hint != "" {
			m["hint"] = d.Hint
		}
		if d.Source != nil {
			src := map[string]any{}
			if d.Source.File != "" {
				src["file"] = d.Source.File
			}
			if d.Source.Pointer != "" {
				src["pointer"] = d.Source.Pointer
			}
			if d.Source.Line > 0 {
				src["line"] = d.Source.Line
			}
			if d.Source.Column > 0 {
				src["column"] = d.Source.Column
			}
			if len(src) > 0 {
				m["source"] = src
			}
		}
		out = append(out, m)
	}
	return out
}

// FormatText renders human-readable diagnostics for stderr.
func FormatText(diags []Diagnostic) string {
	if len(diags) == 0 {
		return ""
	}
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(string(d.Severity))
		b.WriteString(": ")
		b.WriteString(d.Code)
		b.WriteString(": ")
		b.WriteString(d.Message)
		if d.Source != nil {
			if d.Source.File != "" || d.Source.Line > 0 {
				b.WriteString(" (")
				if d.Source.File != "" {
					b.WriteString(d.Source.File)
				}
				if d.Source.Line > 0 {
					if d.Source.File != "" {
						b.WriteString(":")
					}
					b.WriteString(itoa(d.Source.Line))
					if d.Source.Column > 0 {
						b.WriteByte(':')
						b.WriteString(itoa(d.Source.Column))
					}
				}
				if d.Source.Pointer != "" {
					b.WriteString(" @ ")
					b.WriteString(d.Source.Pointer)
				}
				b.WriteByte(')')
			} else if d.Source.Pointer != "" {
				b.WriteString(" (")
				b.WriteString(d.Source.Pointer)
				b.WriteByte(')')
			}
		}
		if d.Hint != "" {
			b.WriteString("\n  hint: ")
			b.WriteString(d.Hint)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
