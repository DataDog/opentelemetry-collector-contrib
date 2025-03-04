// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package datadogfleetautomationextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogfleetautomationextension"

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "Valid configuration",
			config: Config{
				API: APIConfig{
					Site: DefaultSite,
					Key:  "1234567890abcdef1234567890abcdef",
				},
				ReporterPeriod: defaultReporterPeriod,
			},
			wantErr: nil,
		},
		{
			name: "Empty site",
			config: Config{
				API: APIConfig{
					Site: "",
					Key:  "1234567890abcdef1234567890abcdef",
				},
				ReporterPeriod: defaultReporterPeriod,
			},
			wantErr: ErrEmptyEndpoint,
		},
		{
			name: "Unset API key",
			config: Config{
				API: APIConfig{
					Site: DefaultSite,
					Key:  "",
				},
				ReporterPeriod: defaultReporterPeriod,
			},
			wantErr: ErrUnsetAPIKey,
		},
		{
			name: "Invalid API key characters",
			config: Config{
				API: APIConfig{
					Site: DefaultSite,
					Key:  "1234567890abcdef1234567890abcdeg",
				},
				ReporterPeriod: defaultReporterPeriod,
			},
			wantErr: fmt.Errorf("%w: invalid characters: %s", ErrAPIKeyFormat, "g"),
		},
		{
			name: "too small reporter period",
			config: Config{
				API: APIConfig{
					Site: DefaultSite,
					Key:  "1234567890abcdef1234567890abcdef",
				},
				ReporterPeriod: 2 * time.Minute,
			},
			wantErr: fmt.Errorf("reporter_period must be 5 minutes or higher"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
