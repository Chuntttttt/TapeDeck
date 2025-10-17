// Package version provides build-time version information injected via ldflags.
package version

// Build-time variables injected via -ldflags
var (
	// Version is the semantic version (e.g., "v1.0.0")
	Version = "dev"
	// GitCommit is the git commit SHA
	GitCommit = "unknown"
	// BuildDate is the build timestamp
	BuildDate = "unknown"
)

// Info returns formatted version information
type Info struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Get returns the current version information
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
	}
}

// ShortCommit returns the first 8 characters of the commit SHA
func (i Info) ShortCommit() string {
	if len(i.GitCommit) > 8 {
		return i.GitCommit[:8]
	}
	return i.GitCommit
}
