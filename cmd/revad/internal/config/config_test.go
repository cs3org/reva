package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadGlobalGRPCAddress(t *testing.T) {
	config := `
[grpc]
address = "localhost:9142"

[[grpc.services.authprovider]]
driver = "demo"
address = "localhost:9000"

[grpc.services.authprovider.drivers.demo]
key = "value"

[[grpc.services.authprovider]]
driver = "machine"
address = "localhost:9001"

[grpc.services.authprovider.drivers.machine]
key = "value"

[grpc.services.gateway]
something = "test"`

	c, err := Load(strings.NewReader(config))
	if err != nil {
		t.Fatalf("not expected error: %v", err)
	}

	assert.Equal(t, "localhost:9142", c.GRPC.Address)

	exp := map[string]ServicesConfig{
		"authprovider": []*DriverConfig{
			{
				Address: "localhost:9000",
				Config: map[string]any{
					"driver": "demo",
					"drivers": map[string]any{
						"demo": map[string]any{
							"key": "value",
						},
					},
					"address": "localhost:9000",
				},
			},
			{
				Address: "localhost:9001",
				Config: map[string]any{
					"driver":  "machine",
					"address": "localhost:9001",
					"drivers": map[string]any{
						"machine": map[string]any{
							"key": "value",
						},
					},
				},
			},
		},
		"gateway": []*DriverConfig{
			{
				Address: "localhost:9142",
				Config: map[string]any{
					"something": "test",
				},
			},
		},
	}
	assert.Equal(t, exp, c.GRPC._services)
}

func TestLoadNoGRPCDefaultAddress(t *testing.T) {
	config := `
[[grpc.services.authprovider]]
driver = "demo"
address = "localhost:9000"

[grpc.services.authprovider.drivers.demo]
key = "value"

[[grpc.services.authprovider]]
driver = "machine"
address = "localhost:9001"

[grpc.services.authprovider.drivers.machine]
key = "value"

[grpc.services.gateway]
something = "test"`

	c, err := Load(strings.NewReader(config))
	if err != nil {
		t.Fatalf("not expected error: %v", err)
	}

	assert.Equal(t, "", c.GRPC.Address)

	exp := map[string]ServicesConfig{
		"authprovider": []*DriverConfig{
			{
				Address: "localhost:9000",
				Config: map[string]any{
					"driver": "demo",
					"drivers": map[string]any{
						"demo": map[string]any{
							"key": "value",
						},
					},
					"address": "localhost:9000",
				},
			},
			{
				Address: "localhost:9001",
				Config: map[string]any{
					"driver":  "machine",
					"address": "localhost:9001",
					"drivers": map[string]any{
						"machine": map[string]any{
							"key": "value",
						},
					},
				},
			},
		},
		"gateway": []*DriverConfig{
			{
				Config: map[string]any{
					"something": "test",
				},
			},
		},
	}
	assert.Equal(t, exp, c.GRPC._services)
}
