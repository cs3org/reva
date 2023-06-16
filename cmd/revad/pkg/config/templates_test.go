package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyTemplate(t *testing.T) {
	cfg1 := &Config{
		GRPC: &GRPC{
			Services: map[string]ServicesConfig{
				"authprovider": {
					{
						Address: "localhost:1900",
					},
				},
				"authregistry": {
					{
						Address: "localhost:1901",
						Config: map[string]any{
							"drivers": map[string]any{
								"static": map[string]any{
									"demo": "{{ grpc.services.authprovider.address }}",
								},
							},
						},
					},
				},
			},
		},
	}
	err := cfg1.ApplyTemplates(cfg1)
	assert.ErrorIs(t, err, nil)
	assert.Equal(t, cfg1.GRPC.Services["authregistry"][0].Config["drivers"].(map[string]any)["static"].(map[string]any)["demo"], "localhost:1900")

	cfg2 := &Config{
		Vars: Vars{
			"db_username": "root",
			"db_password": "secretpassword",
		},
		GRPC: &GRPC{
			Services: map[string]ServicesConfig{
				"authregistry": {
					{
						Address: "localhost:1901",
						Config: map[string]any{
							"drivers": map[string]any{
								"sql": map[string]any{
									"db_username": "{{ vars.db_username }}",
									"db_password": "{{ vars.db_password }}",
									"key":         "value",
								},
							},
						},
					},
				},
			},
		},
	}

	err = cfg2.ApplyTemplates(cfg2)
	assert.ErrorIs(t, err, nil)
	assert.Equal(t, cfg2.GRPC.Services["authregistry"][0].Config["drivers"].(map[string]any)["sql"],
		map[string]any{
			"db_username": "root",
			"db_password": "secretpassword",
			"key":         "value",
		})

}
