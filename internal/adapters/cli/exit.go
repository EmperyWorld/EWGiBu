package cli

// Exit codes from TZ.md §14.4.
const (
	ExitOK        = 0
	ExitUsage     = 2
	ExitInvalid   = 3
	ExitIO        = 4
	ExitBundle    = 5
	ExitPreflight = 6
	ExitPartial   = 7
	ExitInternal  = 70
)
