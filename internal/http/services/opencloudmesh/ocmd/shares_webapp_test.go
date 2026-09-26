// Copyright 2018-2026 CERN
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

package ocmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
)

func webappReceiveHandler(targets []string) *sharesHandler {
	copied := append([]string{}, targets...)
	return &sharesHandler{
		webappReceiveTargets: &copied,
		ocmClient:            NewClient(10*time.Second, false),
	}
}

func postShareLogged(t *testing.T, h *sharesHandler, body map[string]any, logs *bytes.Buffer) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/ocm/shares", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.15:12345"
	if logs != nil {
		logger := zerolog.New(logs)
		req = req.WithContext(appctx.WithLogger(req.Context(), &logger))
	}
	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)
	return rr
}

func postShare(t *testing.T, h *sharesHandler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return postShareLogged(t, h, body, nil)
}

func shareBody(sender, resourceType string, protocol map[string]any) map[string]any {
	return map[string]any{
		"shareWith":    "marie@local.example.org",
		"name":         "test.txt",
		"providerId":   "provider-id",
		"owner":        "einstein@" + sender,
		"sender":       "einstein@" + sender,
		"shareType":    "user",
		"resourceType": resourceType,
		"protocol":     protocol,
	}
}

func webappOffer(uri string, requirements, targets []string) map[string]any {
	return map[string]any{"webapp": map[string]any{
		"uri":          uri,
		"sharedSecret": "secret",
		"permissions":  []string{"read"},
		"requirements": requirements,
		"targets":      targets,
	}}
}

func capableDiscovery(r *http.Request) wellknown.OcmDiscoveryData {
	return wellknown.OcmDiscoveryData{
		Enabled:       true,
		Endpoint:      "http://" + r.Host + "/ocm/",
		Capabilities:  []string{"exchange-token"},
		TokenEndPoint: "token",
		ResourceTypes: []wellknown.ResourceTypes{{
			Name: "file",
			Protocols: map[string]any{
				"webapp": map[string]any{},
				"webdav": "/remote.php/dav/ocm",
			},
		}},
	}
}

func countingDiscovery(t *testing.T, payload func(*http.Request) wellknown.OcmDiscoveryData) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload(r))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestCreateShareWebappIngest(t *testing.T) {
	srv, hits := countingDiscovery(t, capableDiscovery)
	sender := srv.Listener.Addr().String()
	okCreate := &ocmincoming.CreateOCMIncomingShareResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}

	t.Run("absolute https is stored unchanged", func(t *testing.T) {
		before := hits.Load()
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		const uri = "https://app.example/hub?x=1#y"
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", webappOffer(
			uri,
			[]string{"must-exchange-token"},
			[]string{"blank"},
		)))
		if rr.Code != http.StatusCreated {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if gw.createCalls != 1 || gw.created == nil {
			t.Fatalf("create calls %d", gw.createCalls)
		}
		opts := gw.created.Protocols[0].GetWebappOptions()
		if opts == nil || opts.Uri != uri {
			t.Fatalf("stored uri %#v", opts)
		}
		if hits.Load()-before != 1 {
			t.Fatalf("discovery hits %d", hits.Load()-before)
		}
	})

	t.Run("absolute http is stored unchanged", func(t *testing.T) {
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		const uri = "http://app.example/hub"
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", webappOffer(
			uri,
			[]string{"must-exchange-token"},
			[]string{"blank"},
		)))
		if rr.Code != http.StatusCreated {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		opts := gw.created.Protocols[0].GetWebappOptions()
		if opts == nil || opts.Uri != uri {
			t.Fatalf("stored uri %#v", opts)
		}
	})

	t.Run("empty and padded app names are preserved", func(t *testing.T) {
		for _, appName := range []string{"", " Jupyter "} {
			gw := &sharesMockGW{createResp: okCreate}
			stampGateway(gw)
			offer := webappOffer("https://app.example/hub", []string{"must-exchange-token"}, []string{"blank"})
			offer["webapp"].(map[string]any)["appName"] = appName
			rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", offer))
			if rr.Code != http.StatusCreated {
				t.Fatalf("appName %q status %d body %s", appName, rr.Code, rr.Body.String())
			}
			opts := gw.created.Protocols[0].GetWebappOptions()
			if opts == nil || opts.AppName != appName {
				t.Fatalf("appName stored %#v want %q", opts, appName)
			}
		}
	})

	cases := []struct {
		name         string
		protocol     map[string]any
		want         string
		exact        string
		wantStatus   int
		discoverZero bool
	}{
		{
			name:         "empty targets",
			protocol:     webappOffer("https://app.example/hub", []string{"must-exchange-token"}, []string{}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "no intersection",
			protocol:     webappOffer("https://app.example/hub", []string{"must-exchange-token"}, []string{"iframe"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "mixed offer keeps blank intersection",
			protocol:     webappOffer("https://app.example/hub", []string{"must-exchange-token"}, []string{"iframe", "blank"}),
			want:         "",
			wantStatus:   http.StatusCreated,
			discoverZero: false,
		},
		{
			name:         "missing uri",
			protocol:     webappOffer(" ", []string{"must-exchange-token"}, []string{"blank"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name: "missing secret",
			protocol: map[string]any{"webapp": map[string]any{
				"uri": "https://app.example/hub", "sharedSecret": "",
				"permissions": []string{"read"}, "requirements": []string{"must-exchange-token"},
				"targets": []string{"blank"},
			}},
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "malformed requirement",
			protocol:     webappOffer("https://app.example/hub", []string{" must-exchange-token"}, []string{"blank"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "blank requirement",
			protocol:     webappOffer("https://app.example/hub", []string{" "}, []string{"blank"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "absent must-exchange-token",
			protocol:     webappOffer("https://app.example/hub", []string{"must-use-mfa"}, []string{"blank"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "malformed uri",
			protocol:     webappOffer("http://http://evil.example/hub", []string{"must-exchange-token"}, []string{"blank"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "network path uri",
			protocol:     webappOffer("//evil.example/hub", []string{"must-exchange-token"}, []string{"blank"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "userinfo uri",
			protocol:     webappOffer("https://user:pass@app.example/hub", []string{"must-exchange-token"}, []string{"blank"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "relative uri is not resolved",
			protocol:     webappOffer("/hub", []string{"must-exchange-token"}, []string{"blank"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: false,
		},
		{
			name:         "unknown mandatory requirement",
			protocol:     webappOffer("https://app.example/hub", []string{"must-exchange-token", "must-sign"}, []string{"blank"}),
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name:         "must-use-mfa",
			protocol:     webappOffer("https://app.example/hub", []string{"must-exchange-token", "must-use-mfa"}, []string{"blank"}),
			exact:        ErrWebappMFAUnproven.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
		{
			name: "mixed webdav and bad webapp",
			protocol: map[string]any{
				"webdav": map[string]any{
					"sharedSecret": "secret", "permissions": []string{"read"},
					"uri": "https://dav.example/file",
				},
				"webapp": map[string]any{
					"uri": "https://app.example/hub", "sharedSecret": "secret",
					"permissions": []string{"read"}, "requirements": []string{"must-exchange-token"},
					"targets": []string{"iframe"},
				},
			},
			want:         errInvalidShareRequest.Error(),
			wantStatus:   http.StatusBadRequest,
			discoverZero: true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			before := hits.Load()
			gw := &sharesMockGW{createResp: okCreate}
			stampGateway(gw)
			rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", tt.protocol))
			if rr.Code != tt.wantStatus {
				t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
			}
			if tt.wantStatus == http.StatusCreated {
				if gw.createCalls != 1 {
					t.Fatalf("create calls %d", gw.createCalls)
				}
				return
			}
			if gw.createCalls != 0 {
				t.Fatalf("create calls %d body %s", gw.createCalls, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "INVALID_PARAMETER") {
				t.Fatalf("class missing from %s", rr.Body.String())
			}
			want := tt.want
			if tt.exact != "" {
				want = tt.exact
			}
			if !strings.Contains(rr.Body.String(), want) {
				t.Fatalf("body %s", rr.Body.String())
			}
			if tt.exact != "" && !strings.Contains(rr.Body.String(), ErrWebappMFAUnproven.Error()) {
				t.Fatalf("mfa sentence %s", rr.Body.String())
			}
			if tt.discoverZero && hits.Load() != before {
				t.Fatalf("discovery hits %d", hits.Load()-before)
			}
		})
	}

	t.Run("nil webapp does not discover or save", func(t *testing.T) {
		before := hits.Load()
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", map[string]any{
			"webapp": nil,
		}))
		if rr.Code == http.StatusCreated || gw.createCalls != 0 {
			t.Fatalf("status %d calls %d body %s", rr.Code, gw.createCalls, rr.Body.String())
		}
		if hits.Load() != before {
			t.Fatalf("discovery hits %d", hits.Load()-before)
		}
	})

	t.Run("duplicate webapp does not discover or save", func(t *testing.T) {
		before := hits.Load()
		h := webappReceiveHandler([]string{"blank"})
		first := &Webapp{
			URI: "https://app.example/hub", SharedSecret: "secret",
			Permissions: []string{"read"}, Requirements: []string{"must-exchange-token"},
			Targets: []string{"blank"},
		}
		second := &Webapp{
			URI: "https://app.example/other", SharedSecret: "secret",
			Permissions: []string{"read"}, Requirements: []string{"must-exchange-token"},
			Targets: []string{"blank"},
		}
		protos, legacy, err := h.getAndResolveProtocols(context.Background(), Protocols{first, second}, "file", sender)
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("err %v", err)
		}
		if protos != nil || legacy {
			t.Fatalf("protocols %#v legacy %v", protos, legacy)
		}
		if hits.Load() != before {
			t.Fatalf("discovery hits %d", hits.Load()-before)
		}
	})

	t.Run("typed nil webapp does not discover", func(t *testing.T) {
		before := hits.Load()
		h := webappReceiveHandler([]string{"blank"})
		_, _, err := h.getAndResolveProtocols(context.Background(), Protocols{(*Webapp)(nil)}, "file", sender)
		if err == nil || !strings.Contains(err.Error(), "nil webapp") {
			t.Fatalf("err %v", err)
		}
		if hits.Load() != before {
			t.Fatalf("discovery hits %d", hits.Load()-before)
		}
	})

	t.Run("webdav only still persists", func(t *testing.T) {
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler(nil), shareBody(sender, "file", map[string]any{
			"webdav": map[string]any{
				"sharedSecret": "secret",
				"permissions":  []string{"read"},
				"uri":          "https://dav.example/file.txt",
			},
		}))
		if rr.Code != http.StatusCreated {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if gw.createCalls != 1 {
			t.Fatalf("create calls %d", gw.createCalls)
		}
		if gw.created == nil || gw.created.Protocols[0].GetWebdavOptions() == nil {
			t.Fatal("expected webdav protocol to be stored")
		}
		if gw.created.Protocols[0].GetWebappOptions() != nil {
			t.Fatal("webdav share stored a webapp protocol")
		}
	})
}

func TestCreateShareWebappTokenEndpoint(t *testing.T) {
	okCreate := &ocmincoming.CreateOCMIncomingShareResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}
	offer := func() map[string]any {
		return webappOffer("https://app.example/hub", []string{"must-exchange-token"}, []string{"blank"})
	}

	t.Run("relative token endpoint allows store", func(t *testing.T) {
		srv, hits := countingDiscovery(t, func(r *http.Request) wellknown.OcmDiscoveryData {
			disco := capableDiscovery(r)
			disco.TokenEndPoint = "token"
			return disco
		})
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(srv.Listener.Addr().String(), "file", offer()))
		if rr.Code != http.StatusCreated || gw.createCalls != 1 {
			t.Fatalf("status %d calls %d body %s", rr.Code, gw.createCalls, rr.Body.String())
		}
		opts := gw.created.Protocols[0].GetWebappOptions()
		if opts == nil || opts.Uri != "https://app.example/hub" {
			t.Fatalf("stored %#v", opts)
		}
		if hits.Load() != 1 {
			t.Fatalf("discovery hits %d", hits.Load())
		}
	})

	t.Run("missing exchange-token capability rejects before store", func(t *testing.T) {
		srv, hits := countingDiscovery(t, func(r *http.Request) wellknown.OcmDiscoveryData {
			disco := capableDiscovery(r)
			disco.Capabilities = nil
			disco.TokenEndPoint = "https://token.example/ocm/token"
			return disco
		})
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(srv.Listener.Addr().String(), "file", offer()))
		if rr.Code == http.StatusCreated || gw.createCalls != 0 {
			t.Fatalf("status %d calls %d body %s", rr.Code, gw.createCalls, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), errInvalidShareRequest.Error()) {
			t.Fatalf("body %s", rr.Body.String())
		}
		if hits.Load() != 1 {
			t.Fatalf("discovery hits %d", hits.Load())
		}
	})

	t.Run("userinfo token endpoint rejects before store", func(t *testing.T) {
		srv, _ := countingDiscovery(t, func(r *http.Request) wellknown.OcmDiscoveryData {
			disco := capableDiscovery(r)
			disco.TokenEndPoint = "https://user:pass@token.example/token"
			return disco
		})
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(srv.Listener.Addr().String(), "file", offer()))
		if rr.Code == http.StatusCreated || gw.createCalls != 0 {
			t.Fatalf("status %d calls %d body %s", rr.Code, gw.createCalls, rr.Body.String())
		}
		if strings.Contains(rr.Body.String(), "user:pass") {
			t.Fatalf("body leaked userinfo: %s", rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), errInvalidShareRequest.Error()) {
			t.Fatalf("body %s", rr.Body.String())
		}
	})
}

func TestCreateShareParserErrorOmitsSecret(t *testing.T) {
	const secret = "super-secret-share-token"
	h := webappReceiveHandler([]string{"blank"})
	var logs bytes.Buffer
	req := httptest.NewRequest(http.MethodPost, "/ocm/shares", bytes.NewReader([]byte(`{"sharedSecret":"`+secret)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.15:12345"
	logger := zerolog.New(&logs)
	req = req.WithContext(appctx.WithLogger(req.Context(), &logger))
	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), errInvalidShareRequest.Error()) {
		t.Fatalf("body %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "INVALID_PARAMETER") {
		t.Fatalf("class missing from %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), secret) || strings.Contains(logs.String(), secret) {
		t.Fatalf("secret leaked response %s logs %s", rr.Body.String(), logs.String())
	}
}

func TestCreateShareWebappValidationErrorsAreSafe(t *testing.T) {
	srv, hits := countingDiscovery(t, capableDiscovery)
	sender := srv.Listener.Addr().String()
	okCreate := &ocmincoming.CreateOCMIncomingShareResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}
	const (
		paddedRequirement  = " padded-req-ATTACK"
		blankRequirement   = "\t\t"
		unknownRequirement = "req-ATTACK-unknown"
		unknownTarget      = "target-ATTACK-popup"
		unknownPermission  = "perm-ATTACK-admin"
	)
	permissionOffer := webappOffer(
		"https://app.example/hub",
		[]string{"must-exchange-token"},
		[]string{"blank"},
	)
	permissionOffer["webapp"].(map[string]any)["permissions"] = []string{unknownPermission}
	cases := []struct {
		name      string
		protocol  map[string]any
		classText string
		forbidden []string
	}{
		{
			name:      "padded requirement",
			protocol:  webappOffer("https://app.example/hub", []string{paddedRequirement}, []string{"blank"}),
			classText: "malformed requirement",
			forbidden: []string{paddedRequirement, "padded-req-ATTACK"},
		},
		{
			name:      "blank requirement",
			protocol:  webappOffer("https://app.example/hub", []string{blankRequirement}, []string{"blank"}),
			classText: "malformed requirement",
			forbidden: []string{blankRequirement},
		},
		{
			name:      "unknown requirement",
			protocol:  webappOffer("https://app.example/hub", []string{"must-exchange-token", unknownRequirement}, []string{"blank"}),
			classText: "unsupported requirement",
			forbidden: []string{unknownRequirement},
		},
		{
			name:      "unknown target",
			protocol:  webappOffer("https://app.example/hub", []string{"must-exchange-token"}, []string{unknownTarget}),
			classText: "unsupported target",
			forbidden: []string{unknownTarget},
		},
		{
			name:      "unknown permission",
			protocol:  permissionOffer,
			classText: "unsupported permission",
			forbidden: []string{unknownPermission},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.protocol)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Protocols
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			verr := decoded.Validate()
			if verr == nil || !strings.Contains(verr.Error(), tt.classText) {
				t.Fatalf("validator error %v", verr)
			}
			screenErr := ScreenIncomingWebapps(decoded, []string{"blank"})
			if screenErr == nil || !strings.Contains(screenErr.Error(), tt.classText) {
				t.Fatalf("screen error %v", screenErr)
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(verr.Error(), forbidden) || strings.Contains(screenErr.Error(), forbidden) {
					t.Fatalf("validator leaked %q validate %v screen %v", forbidden, verr, screenErr)
				}
			}

			before := hits.Load()
			gw := &sharesMockGW{createResp: okCreate}
			stampGateway(gw)
			var logs bytes.Buffer
			rr := postShareLogged(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", tt.protocol), &logs)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
			}
			body := rr.Body.String()
			if !strings.Contains(body, "INVALID_PARAMETER") {
				t.Fatalf("class missing from %s", body)
			}
			if !strings.Contains(body, errInvalidShareRequest.Error()) {
				t.Fatalf("body %s", body)
			}
			logged := logs.String()
			if !strings.Contains(logged, errInvalidShareRequest.Error()) {
				t.Fatalf("log %s", logged)
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(body, forbidden) || strings.Contains(logged, forbidden) {
					t.Fatalf("leaked %q body %s logs %s", forbidden, body, logged)
				}
			}
			if gw.createCalls != 0 {
				t.Fatalf("create calls %d body %s", gw.createCalls, body)
			}
			if hits.Load() != before {
				t.Fatalf("discovery hits %d", hits.Load()-before)
			}
		})
	}
}
