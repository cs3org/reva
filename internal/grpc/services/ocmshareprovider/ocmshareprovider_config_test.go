// Copyright 2018-2024 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package ocmshareprovider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/embedded"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func TestOfferWebappConfigDecodeAndDisabledStartup(t *testing.T) {
	var decoded config
	err := cfg.Decode(map[string]any{
		"gatewaysvc":      "127.0.0.1:9142",
		"provider_domain": "sender.example",
		"webdav_endpoint": testWebDAVRoot,
		"offer_webapp":    true,
		"webapp_name":     testPaddedName,
		"webapp_endpoint": testOpener,
		"enable_webapp":   false,
	}, &decoded)
	if err != nil {
		t.Fatalf("decode enabled offer: %v", err)
	}
	if !decoded.OfferWebapp || decoded.WebappName != testPaddedName || decoded.WebAppEndpoint != testOpener {
		t.Fatalf("decoded config = %#v", decoded)
	}

	var defaults config
	err = cfg.Decode(map[string]any{
		"gatewaysvc":      "127.0.0.1:9142",
		"provider_domain": "sender.example",
		"webdav_endpoint": testWebDAVRoot,
		"enable_webapp":   true,
	}, &defaults)
	if err != nil {
		t.Fatalf("decode disabled offer: %v", err)
	}
	if defaults.OfferWebapp || defaults.WebappName != "" || defaults.WebAppEndpoint != "" {
		t.Fatalf("disabled defaults = %#v, want false and empty fields", defaults)
	}

	svc, err := New(context.Background(), map[string]any{
		"driver":          testDriverName,
		"embedded_driver": testDriverName,
		"gatewaysvc":      "127.0.0.1:9142",
		"provider_domain": "sender.example",
		"webdav_endpoint": testWebDAVRoot,
		"enable_webapp":   true,
		"webapp_name":     "   ",
		"webapp_endpoint": "http://[",
	})
	if err != nil {
		t.Fatalf("disabled New: %v", err)
	}
	got := svc.(*service)
	if got.conf.OfferWebapp || got.conf.WebappName != "   " || got.conf.WebAppEndpoint != "http://[" {
		t.Fatalf("disabled service config = %#v", got.conf)
	}
}

func TestOfferWebappEnabledStartup(t *testing.T) {
	svc, err := New(context.Background(), map[string]any{
		"driver":          testDriverName,
		"embedded_driver": testDriverName,
		"gatewaysvc":      "127.0.0.1:9142",
		"provider_domain": "sender.example",
		"webdav_endpoint": testWebDAVRoot,
		"offer_webapp":    true,
		"webapp_name":     testPaddedName,
		"webapp_endpoint": testOpener,
	})
	if err != nil {
		t.Fatalf("enabled New: %v", err)
	}
	if svc == nil {
		t.Fatal("enabled New returned nil service")
	}
	got := svc.(*service)
	if !got.conf.OfferWebapp || got.conf.WebappName != testPaddedName || got.conf.WebAppEndpoint != testOpener {
		t.Fatalf("enabled service config = %#v", got.conf)
	}
}

func TestOfferWebappInitializationRejectsInvalidConfig(t *testing.T) {
	var driverCalled bool
	withTestDrivers(t, testNoRunDriver,
		func(context.Context, map[string]any) (share.Repository, error) {
			driverCalled = true
			return &capturingRepo{}, nil
		},
		func(context.Context, map[string]any) (embedded.Transferrer, error) {
			return noopTransferrer{}, nil
		},
	)

	cases := []struct {
		name   string
		cfg    map[string]any
		substr string
	}{
		{
			name: "missing name",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_endpoint": testOpener,
			},
			substr: "webapp_name",
		},
		{
			name: "blank name",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     "   ",
				"webapp_endpoint": testOpener,
			},
			substr: "webapp_name",
		},
		{
			name: "missing endpoint",
			cfg: map[string]any{
				"offer_webapp": true,
				"webapp_name":  testAppName,
			},
			substr: "webapp_endpoint",
		},
		{
			name: "relative endpoint",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     testAppName,
				"webapp_endpoint": "/services/ocm/open",
			},
			substr: "webapp_endpoint",
		},
		{
			name: "hostless endpoint",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     testAppName,
				"webapp_endpoint": "http:///open",
			},
			substr: "webapp_endpoint",
		},
		{
			name: "endpoint without hostname",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     testAppName,
				"webapp_endpoint": "http://:8080/open",
			},
			substr: "webapp_endpoint",
		},
		{
			name: "malformed endpoint",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     testAppName,
				"webapp_endpoint": "http://[",
			},
			substr: "webapp_endpoint",
		},
		{
			name: "endpoint with userinfo",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     testAppName,
				"webapp_endpoint": "https://user:pass@provider.example/services/ocm/open",
			},
			substr: "http or https URL with a hostname and no userinfo",
		},
		{
			name: "non-http endpoint",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     testAppName,
				"webapp_endpoint": "ftp://provider.example/services/ocm/open",
			},
			substr: "http or https",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			driverCalled = false
			input := map[string]any{
				"driver":          testNoRunDriver,
				"embedded_driver": testNoRunDriver,
				"gatewaysvc":      "127.0.0.1:9142",
				"provider_domain": "sender.example",
				"webdav_endpoint": testWebDAVRoot,
			}
			for k, v := range tt.cfg {
				input[k] = v
			}
			svc, err := New(context.Background(), input)
			if err == nil {
				t.Fatal("expected initialization error")
			}
			if svc != nil {
				t.Fatal("expected no service on invalid config")
			}
			var bad errtypes.BadRequest
			if !errors.As(err, &bad) {
				t.Fatalf("error %T %v, want BadRequest", err, err)
			}
			var missing errtypes.NotFound
			if errors.As(err, &missing) {
				t.Fatalf("invalid config returned NotFound: %v", err)
			}
			if !strings.Contains(err.Error(), tt.substr) {
				t.Fatalf("error %q does not mention %s", err.Error(), tt.substr)
			}
			if driverCalled {
				t.Fatal("invalid config reached the share repository")
			}
		})
	}
}
