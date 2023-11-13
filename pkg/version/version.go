package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the current version (updated by the build).
	Version = "0.0.0-dev"
	// GitCommit is the current Git commit SHA (updated by the build).
	GitCommit string
	// BuildDate is the current build date (updated by the build).
	BuildDate = "1970-01-01T00:00:00Z"
)

// Get returns the overall codebase version.
func Get() string {
	return fmt.Sprintf(`version : %s
git commit  : %s
build date  : %s
go version  : %s
go compiler : %s
platform    : %s/%s`, Version, GitCommit, BuildDate, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
}
