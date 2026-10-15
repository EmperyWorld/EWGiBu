package cli

import (
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/versioninfo"
)

func runVersion(args []string, stdout, stderr io.Writer) int {
	output, rest, err := parseOutputFlag(args)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitUsage
	}
	if len(rest) > 0 {
		a := rest[0]
		if a == "-h" || a == "--help" {
			fmt.Fprintln(stdout, "usage: iacpack version [--output text|json]")
			return ExitOK
		}
		fmt.Fprintf(stderr, "unexpected argument %q\n", a)
		return ExitUsage
	}

	info := versioninfo.Current()
	switch strings.ToLower(output) {
	case "", "text":
		if _, err := fmt.Fprint(stdout, formatVersionText(info)); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return ExitIO
		}
		return ExitOK
	case "json":
		if err := WriteJSON(stdout, "version", true, info, nil); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return ExitInternal
		}
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unsupported --output %q (allowed: text, json)\n", output)
		return ExitUsage
	}
}

func formatVersionText(info versioninfo.Info) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", info.Product, info.Version)
	fmt.Fprintf(&b, "goVersion: %s\n", info.GoVersion)
	fmt.Fprintf(&b, "commit: %s\n", info.Commit)
	fmt.Fprintf(&b, "dirty: %t\n", info.Dirty)
	return b.String()
}
