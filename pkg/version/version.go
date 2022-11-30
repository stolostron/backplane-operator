// Copyright Contributors to the Open Cluster Management project

package version

import (
	"fmt"
	"os"
	"runtime"

	"github.com/Masterminds/semver"
)

// Version is the semver version the operator is reconciling towards
var Version string

// MinimumOCPVersion is the minimum version of OCP this operator supports.
// Can be overridden by setting the env variable DISABLE_OCP_MIN_VERSION
var MinimumOCPVersion string = "4.10.0"

func init() {
	if value, exists := os.LookupEnv("OPERATOR_VERSION"); exists {
		Version = value
	} else {
		Version = "9.9.9"
	}
}

// Info contains versioning information.
type Info struct {
	GitVersion   string `json:"gitVersion"`
	GitCommit    string `json:"gitCommit"`
	GitTreeState string `json:"gitTreeState"`
	BuildDate    string `json:"buildDate"`
	GoVersion    string `json:"goVersion"`
	Compiler     string `json:"compiler"`
	Platform     string `json:"platform"`
}

// Get returns the overall codebase version. It's for detecting
// what code a binary was built from.
func Get() Info {
	// These variables typically come from -ldflags settings and in
	// their absence fallback to the settings in pkg/version/base.go
	return Info{
		GitVersion:   gitVersion,
		GitCommit:    gitCommit,
		GitTreeState: gitTreeState,
		BuildDate:    buildDate,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// ValidOCPVersion returns an error if ocpVersion does not satisfy the minimum OCP version requirement
func ValidOCPVersion(ocpVersion string) error {
	if _, exists := os.LookupEnv("DISABLE_OCP_MIN_VERSION"); exists {
		return nil
	}

	aboveMinVersion, err := semver.NewConstraint(fmt.Sprintf(">= %s-0", MinimumOCPVersion))
	if err != nil {
		return err
	}
	currentVersion, err := semver.NewVersion(ocpVersion)
	if err != nil {
		return err
	}
	if !aboveMinVersion.Check(currentVersion) {
		return fmt.Errorf("OCP version %s did not meet minimum version requirement of %s", ocpVersion, MinimumOCPVersion)
	}
	return nil
}
