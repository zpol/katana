package version

import "os"

// Version is set at link time via -ldflags or overridden by KATANA_VERSION.
var Version = "dev"

// Current returns the running KATANA version.
func Current() string {
	if v := os.Getenv("KATANA_VERSION"); v != "" {
		return v
	}
	if Version != "" {
		return Version
	}
	return "dev"
}
