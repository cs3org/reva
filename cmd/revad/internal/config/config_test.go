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
	assert.Equal(t, exp, c.GRPC.Services)
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
	assert.Equal(t, exp, c.GRPC.Services)
}

func TestLoadFullConfig(t *testing.T) {
	config := `
[shared]
gatewaysvc = "localhost:9142"
jwt_secret = "secret"

[log]
output = "/var/log/revad/revad-gateway.log"
mode = "json"
level = "trace"

[core]
max_cpus = 1
tracing_enabled = true

[vars]
db_username = "root"
db_password = "secretpassword"

[grpc]
shutdown_deadline = 10
enable_reflection = true

[grpc.services.gateway]
authregistrysvc = "{{ grpc.services.authregistry.address }}"

[grpc.services.authregistry]
driver = "static"

[grpc.services.authregistry.drivers.static.rules]
basic = "{{ grpc.services.authprovider[0].address }}"
machine = "{{ grpc.services.authprovider[1].address }}"

[[grpc.services.authprovider]]
driver = "ldap"
address = "localhost:19000"

[grpc.services.authprovider.drivers.ldap]
password = "ldap"

[[grpc.services.authprovider]]
driver = "machine"
address = "localhost:19001"

[grpc.services.authprovider.drivers.machine]
api_key = "secretapikey"

[http]
address = "localhost:19002"

[http.services.dataprovider]
driver = "localhome"

[http.services.sysinfo]

[serverless.services.notifications]
nats_address = "nats-server-01.example.com"
nats_token = "secret-token-example"`

	c2, err := Load(strings.NewReader(config))
	assert.ErrorIs(t, err, nil)

	assert.Equal(t, c2.Shared, &Shared{
		GatewaySVC: "localhost:9142",
		JWTSecret:  "secret",
	})

	assert.Equal(t, c2.Log, &Log{
		Output: "/var/log/revad/revad-gateway.log",
		Mode:   "json",
		Level:  "trace",
	})

	assert.Equal(t, c2.Core, &Core{
		MaxCPUs:        1,
		TracingEnabled: true,
	})

	assert.Equal(t, c2.Vars, Vars{
		"db_username": "root",
		"db_password": "secretpassword",
	})

	assertGRPCEqual(t, c2.GRPC, &GRPC{
		ShutdownDeadline: 10,
		EnableReflection: true,
		Interceptors:     make(map[string]map[string]any),
		Services: map[string]ServicesConfig{
			"gateway": {
				{
					Config: map[string]any{
						"authregistrysvc": "{{ grpc.services.authregistry.address }}",
					},
				},
			},
			"authregistry": {
				{
					Config: map[string]any{
						"driver": "static",
						"drivers": map[string]any{
							"static": map[string]any{
								"rules": map[string]any{
									"basic":   "{{ grpc.services.authprovider[0].address }}",
									"machine": "{{ grpc.services.authprovider[1].address }}",
								},
							},
						},
					},
				},
			},
			"authprovider": {
				{
					Address: "localhost:19000",
					Config: map[string]any{
						"driver":  "ldap",
						"address": "localhost:19000",
						"drivers": map[string]any{
							"ldap": map[string]any{
								"password": "ldap",
							},
						},
					},
				},
				{
					Address: "localhost:19001",
					Config: map[string]any{
						"driver":  "machine",
						"address": "localhost:19001",
						"drivers": map[string]any{
							"machine": map[string]any{
								"api_key": "secretapikey",
							},
						},
					},
				},
			},
		},
	})

	assertHTTPEqual(t, c2.HTTP, &HTTP{
		Address:     "localhost:19002",
		Middlewares: make(map[string]map[string]any),
		Services: map[string]ServicesConfig{
			"dataprovider": {
				{
					Address: "localhost:19002",
					Config: map[string]any{
						"driver": "localhome",
					},
				},
			},
			"sysinfo": {
				{
					Address: "localhost:19002",
					Config:  map[string]any{},
				},
			},
		},
	})

	assert.Equal(t, c2.Serverless, &Serverless{
		Services: map[string]map[string]any{
			"notifications": {
				"nats_address": "nats-server-01.example.com",
				"nats_token":   "secret-token-example",
			},
		},
	})
}

func assertGRPCEqual(t *testing.T, g1, g2 *GRPC) {
	assert.Equal(t, g1.Address, g2.Address)
	assert.Equal(t, g1.Network, g2.Network)
	assert.Equal(t, g1.ShutdownDeadline, g2.ShutdownDeadline)
	assert.Equal(t, g1.EnableReflection, g2.EnableReflection)
	assert.Equal(t, g1.Services, g2.Services)
	assert.Equal(t, g1.Interceptors, g2.Interceptors)
}

func assertHTTPEqual(t *testing.T, h1, h2 *HTTP) {
	assert.Equal(t, h1.Network, h2.Network)
	assert.Equal(t, h1.Network, h2.Network)
	assert.Equal(t, h1.CertFile, h2.CertFile)
	assert.Equal(t, h1.KeyFile, h2.KeyFile)
	assert.Equal(t, h1.Services, h2.Services)
	assert.Equal(t, h1.Middlewares, h2.Middlewares)
}
