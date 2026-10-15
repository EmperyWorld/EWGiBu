package importpropose

import (
	"fmt"
	"strconv"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
)

func assignImportedGPU(dst map[string]any, count int, all bool) {
	if dst == nil {
		return
	}
	if all {
		dst["gpu"] = "all"
		return
	}
	if count > 0 {
		dst["gpu"] = count
	}
}

// importDeployGPU maps Compose deploy.resources.reservations.devices (GPU only) to a count.
// Service-level devices: stays policy-denied in mapService (ADR-021).
func importDeployGPU(res *loader.Node, file, pointer, sid string) (count int, all bool, diags []diagnostic.Diagnostic) {
	if res == nil || res.Kind != loader.KindMap {
		return 0, false, nil
	}
	reserv := res.GetMap("reservations")
	if reserv == nil || reserv.Kind != loader.KindMap {
		return 0, false, nil
	}
	devs := reserv.GetMap("devices")
	if devs == nil {
		return 0, false, nil
	}
	if devs.Kind != loader.KindSeq {
		diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("service %s: deploy.resources.reservations.devices omitted; expected a list", sid)).
			WithSource(file, pointer+"/deploy/resources/reservations/devices", 0, 0))
		return 0, false, diags
	}
	for i, item := range devs.Seq {
		n, isAll, ok, d := gpuDeviceCount(item, file, fmt.Sprintf("%s/deploy/resources/reservations/devices/%d", pointer, i), sid)
		diags = append(diags, d...)
		if !ok {
			continue
		}
		if count != 0 || all {
			diags = append(diags, diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("service %s: extra GPU device reservation omitted", sid)).
				WithSource(file, fmt.Sprintf("%s/deploy/resources/reservations/devices/%d", pointer, i), 0, 0))
			continue
		}
		all = isAll
		count = n
	}
	return count, all, diags
}

func gpuDeviceCount(item *loader.Node, file, pointer, sid string) (int, bool, bool, []diagnostic.Diagnostic) {
	if item == nil || item.Kind != loader.KindMap {
		return 0, false, false, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("service %s: GPU device entry omitted", sid)).WithSource(file, pointer, 0, 0)}
	}
	if item.GetMap("device_ids") != nil || item.GetMap("options") != nil {
		return 0, false, false, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportPolicyDenied,
			fmt.Sprintf("service %s: GPU device_ids/options refused; only driver nvidia, count, capabilities [gpu]", sid)).
			WithSource(file, pointer, 0, 0)}
	}
	if drv := item.GetMap("driver"); drv != nil && drv.Kind == loader.KindString {
		if strings.TrimSpace(drv.Str) != "" && !strings.EqualFold(drv.Str, "nvidia") {
			return 0, false, false, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("service %s: GPU driver %q omitted; only nvidia", sid, drv.Str)).
				WithSource(file, pointer+"/driver", 0, 0)}
		}
	}
	if !capabilitiesAreGPUOnly(item.GetMap("capabilities")) {
		return 0, false, false, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("service %s: device reservation omitted; capabilities must be [gpu]", sid)).
			WithSource(file, pointer+"/capabilities", 0, 0)}
	}
	c := item.GetMap("count")
	if c == nil {
		return 0, false, false, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("service %s: GPU reservation omitted; count is required (1..8 or all)", sid)).
			WithSource(file, pointer+"/count", 0, 0)}
	}
	if c.Kind == loader.KindString && strings.EqualFold(strings.TrimSpace(c.Str), "all") {
		return 0, true, true, nil
	}
	n := 0
	switch c.Kind {
	case loader.KindNumber:
		v, err := strconv.Atoi(strings.TrimSpace(c.Str))
		if err != nil {
			return 0, false, false, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("service %s: GPU count omitted", sid)).WithSource(file, pointer+"/count", c.Line, c.Column)}
		}
		n = v
	case loader.KindString:
		v, err := strconv.Atoi(strings.TrimSpace(c.Str))
		if err != nil {
			return 0, false, false, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("service %s: GPU count omitted", sid)).WithSource(file, pointer+"/count", c.Line, c.Column)}
		}
		n = v
	default:
		return 0, false, false, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("service %s: GPU count omitted", sid)).WithSource(file, pointer+"/count", 0, 0)}
	}
	if n < 1 || n > 8 {
		return 0, false, false, []diagnostic.Diagnostic{diagnostic.Warning(diagnostic.CodeImportUnsupportedField,
			fmt.Sprintf("service %s: GPU count %d omitted; allowed 1..8 or all", sid, n)).
			WithSource(file, pointer+"/count", c.Line, c.Column)}
	}
	return n, false, true, nil
}

func capabilitiesAreGPUOnly(n *loader.Node) bool {
	if n == nil {
		return false
	}
	var caps []string
	switch n.Kind {
	case loader.KindString:
		caps = []string{strings.TrimSpace(n.Str)}
	case loader.KindSeq:
		for _, c := range n.Seq {
			if c != nil && c.Kind == loader.KindString {
				caps = append(caps, strings.TrimSpace(c.Str))
			}
		}
	default:
		return false
	}
	if len(caps) != 1 {
		return false
	}
	return strings.EqualFold(caps[0], "gpu")
}
