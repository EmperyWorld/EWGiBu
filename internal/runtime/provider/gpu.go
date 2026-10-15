package provider

import "iacpack/internal/runtime/cdi"

// NVIDIACountAllCDIDevice is the CDI name podman-compose passes for a
// Compose reservation driver nvidia, count all, capabilities [gpu].
const NVIDIACountAllCDIDevice = "nvidia.com/gpu=all"

// cdiSpecDirs is replaced in tests. Production reads host containers.conf.
var cdiSpecDirs = cdi.SpecDirs

// PodmanCountAllCDIMissing reports whether this engine would turn
// count: all into NVIDIACountAllCDIDevice and that device is not declared
// by a host CDI spec. Docker keeps the Compose reservation.
func (h HostApply) PodmanCountAllCDIMissing() bool {
	eng, err := h.resolvedEngine()
	if err != nil || eng != EnginePodman {
		return false
	}
	return !cdi.Declares(cdiSpecDirs(), "nvidia.com/gpu", "all")
}
