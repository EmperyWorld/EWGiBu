// Package buildinfo holds compile-time identity for the iacpack product.
// Values other than the development defaults must be supplied through -ldflags;
// this package never injects the current time or host identity.
package buildinfo

// Product is the stable product name shown in CLI version output.
const Product = "iacpack"

// Version is the product version string for this source tree.
// Override at link time with -X iacpack/internal/buildinfo.Version=...
var Version = "1.1.0"

// Commit is the source revision when supplied through build flags.
// The default development value is "unknown".
var Commit = "unknown"

// Dirty reports whether the build was produced from a dirty work tree when
// that metadata is supplied through build flags. Defaults to false.
var Dirty = "false"

// DirtyBool returns the parsed dirty flag. Unrecognized values are treated as false.
func DirtyBool() bool {
	return Dirty == "true" || Dirty == "1"
}
