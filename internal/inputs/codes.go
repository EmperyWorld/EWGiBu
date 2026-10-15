package inputs

// Stable INPUT.* diagnostic codes (TZ.md §12.3).
const (
	CodePathOutsideWorkspace = "MANIFEST.PATH_OUTSIDE_WORKSPACE"
	CodeInputMissing         = "INPUT.MISSING"
	CodeInputEmpty           = "INPUT.EMPTY_MATCH"
	CodeInputCollision       = "INPUT.PATH_COLLISION"
	CodeInputForbiddenType   = "INPUT.FORBIDDEN_TYPE"
	CodeInputGlob            = "INPUT.INVALID_GLOB"
	CodeInputIO              = "INPUT.IO_ERROR"
	CodeInputChanged         = "INPUT.CHANGED_DURING_HASH"
	CodeInputOutputRecursion = "INPUT.OUTPUT_RECURSION"
	CodeInputPermission      = "INPUT.PERMISSION"
)
