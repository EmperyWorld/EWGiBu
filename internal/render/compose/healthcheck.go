package compose

import "strings"

const (
	healthcheckShellPath = "/bin/sh"
	healthcheckShellFlag = "-c"
)

// HealthcheckTestFromManifest maps manifest argv to Compose healthcheck.test (CMD or CMD-SHELL).
func HealthcheckTestFromManifest(command []string) []string {
	if len(command) == 0 {
		return nil
	}
	if isManifestShellHealthcheck(command) {
		script := strings.TrimSpace(command[2])
		if script == "" {
			return append([]string{"CMD"}, command...)
		}
		return []string{"CMD-SHELL", script}
	}
	out := make([]string, 1, len(command)+1)
	out[0] = "CMD"
	out = append(out, command...)
	return out
}

func isManifestShellHealthcheck(command []string) bool {
	if len(command) < 3 {
		return false
	}
	return command[0] == healthcheckShellPath && command[1] == healthcheckShellFlag
}
