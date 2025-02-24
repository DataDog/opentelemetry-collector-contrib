// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package datadogfleetautomationextension

import (
	"fmt"
	"testing"

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
			},
			wantErr: fmt.Errorf("%w: invalid characters: %s", ErrAPIKeyFormat, "g"),
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
