package importpropose

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// parseComposeMemoryBytes maps Docker Compose deploy.resources.limits.memory to manifest bytes (1 MiB .. 1 TiB).
func parseComposeMemoryBytes(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty memory limit")
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n, nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil && !strings.ContainsAny(raw, "bkmgtBKMTG") {
		if f != math.Trunc(f) {
			return 0, fmt.Errorf("non-integer memory %q", raw)
		}
		return int64(f), nil
	}
	i := len(raw)
	for i > 0 && (raw[i-1] >= 'a' && raw[i-1] <= 'z' || raw[i-1] >= 'A' && raw[i-1] <= 'Z') {
		i--
	}
	if i == len(raw) {
		return 0, fmt.Errorf("unrecognized memory limit %q", raw)
	}
	num := strings.TrimSpace(raw[:i])
	suf := strings.ToLower(strings.TrimSpace(raw[i:]))
	mult, ok := composeMemorySuffix[suf]
	if !ok {
		return 0, fmt.Errorf("unrecognized memory suffix in %q", raw)
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid memory value %q", raw)
	}
	bytes := v * float64(mult)
	if bytes > float64(math.MaxInt64) {
		return 0, fmt.Errorf("memory limit too large %q", raw)
	}
	return int64(bytes), nil
}

var composeMemorySuffix = map[string]int64{
	"b":  1,
	"k":  1 << 10,
	"kb": 1 << 10,
	"m":  1 << 20,
	"mb": 1 << 20,
	"g":  1 << 30,
	"gb": 1 << 30,
	"t":  1 << 40,
	"tb": 1 << 40,
}
