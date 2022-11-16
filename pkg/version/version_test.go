// Copyright Contributors to the Open Cluster Management project

package version

import (
	"fmt"
	"testing"
)

func Test_ValidOCPVersion(t *testing.T) {
	tests := []struct {
		name       string
		ocpVersion string
		envVar     string
		wantErr    bool
	}{
		{
			name:       "above min",
			ocpVersion: "4.99.99",
			wantErr:    false,
		},
		{
			name:       "below min",
			ocpVersion: "4.9.99",
			wantErr:    true,
		},
		{
			name:       "below min ignored",
			ocpVersion: "4.9.99",
			envVar:     "DISABLE_OCP_MIN_VERSION",
			wantErr:    false,
		},
		{
			name:       "no version found",
			ocpVersion: "",
			wantErr:    true,
		},
		{
			name:       "dev version passing",
			ocpVersion: fmt.Sprintf("%s-dev", MinimumOCPVersion),
			wantErr:    false,
		},
		{
			name:       "exact version",
			ocpVersion: MinimumOCPVersion,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVar != "" {
				t.Setenv(tt.envVar, "true")
			}
			if err := ValidOCPVersion(tt.ocpVersion); (err != nil) != tt.wantErr {
				t.Errorf("validOCPVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
