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

package appregistry

import (
	"context"
	"testing"

	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	svcregistry "github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/registry/memory"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

// node describes an app provider as it would appear in the service registry.
type node struct {
	address   string
	app       string
	mimeTypes []string
	priority  string
	state     string
	transport string
	// raw overrides the encoded app description, to simulate a malformed one.
	raw string
}

func (n node) build(t *testing.T) (string, svcregistry.Node) {
	t.Helper()

	encoded := n.raw
	if encoded == "" && n.app != "" {
		info := &registrypb.ProviderInfo{Name: n.app, MimeTypes: n.mimeTypes}
		if n.priority != "" {
			info.Opaque = &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					"priority": {Decoder: "plain", Value: []byte(n.priority)},
				},
			}
		}
		b, err := protojson.Marshal(info)
		require.NoError(t, err)
		encoded = string(b)
	}

	transport := n.transport
	if transport == "" {
		transport = svcregistry.TransportGRPC
	}
	state := n.state
	if state == "" {
		state = svcregistry.StateReady
	}

	meta := map[string]string{
		svcregistry.MetaTransport: transport,
		svcregistry.MetaState:     state,
	}
	if encoded != "" {
		meta[svcregistry.MetaApp] = encoded
	}
	return n.address, svcregistry.NewNode(n.address+"/appprovider", n.address, meta)
}

// newRegistry builds a registry driver over a service registry populated with
// the given app provider nodes.
func testRegistry(t *testing.T, mimeTypes []*mimeTypeConfig, nodes ...node) *manager {
	t.Helper()

	reg := memory.New(nil)
	for _, n := range nodes {
		_, built := n.build(t)
		require.NoError(t, reg.Add(svcregistry.NewService(service.NameAppProvider, []svcregistry.Node{built})))
	}

	m := newRegistry(&config{MimeTypes: mimeTypes})
	m.resolve = func() svcregistry.Registry { return reg }
	// A deterministic pick keeps the assertions on addresses stable.
	m.selector = service.FirstSelector{}
	return m
}

func names(providers []*registrypb.ProviderInfo) []string {
	out := make([]string, 0, len(providers))
	for _, p := range providers {
		out = append(out, p.Name)
	}
	return out
}

var officeMimeTypes = []*mimeTypeConfig{
	{MimeType: "text/json", Extension: "json", Name: "JSON File", DefaultApp: "TextEditor"},
	{MimeType: "image/bmp", Extension: "bmp", Name: "Image File"},
}

func TestFindProviders(t *testing.T) {
	r := testRegistry(t, officeMimeTypes,
		node{address: "text:1000", app: "TextEditor", mimeTypes: []string{"text/json", "text/xml"}},
		node{address: "image:1001", app: "ImageViewer", mimeTypes: []string{"image/bmp"}},
	)

	t.Run("returns the provider handling the mime type", func(t *testing.T) {
		got, err := r.FindProviders(context.Background(), "text/json")
		require.NoError(t, err)
		assert.Equal(t, []string{"TextEditor"}, names(got))
		assert.Equal(t, "text:1000", got[0].Address, "the address must come from the registry node")
	})

	t.Run("a mime type only a provider knows about still resolves", func(t *testing.T) {
		got, err := r.FindProviders(context.Background(), "text/xml")
		require.NoError(t, err)
		assert.Equal(t, []string{"TextEditor"}, names(got))
	})

	t.Run("an unhandled mime type is not found", func(t *testing.T) {
		_, err := r.FindProviders(context.Background(), "doesnot/exist")
		assert.Error(t, err)
	})

	t.Run("an empty mime type is not found", func(t *testing.T) {
		_, err := r.FindProviders(context.Background(), "")
		assert.Error(t, err)
	})
}

func TestFindProvidersOrdersByPriority(t *testing.T) {
	r := testRegistry(t, officeMimeTypes,
		node{address: "low:1000", app: "Low", mimeTypes: []string{"text/json"}, priority: "1"},
		node{address: "high:1001", app: "High", mimeTypes: []string{"text/json"}, priority: "10"},
		node{address: "none:1002", app: "Unset", mimeTypes: []string{"text/json"}},
	)

	got, err := r.FindProviders(context.Background(), "text/json")
	require.NoError(t, err)
	assert.Equal(t, []string{"High", "Low", "Unset"}, names(got))
}

// The bug this design removes: app providers used to be keyed by address, so
// several of them offering the same mime type collapsed into one entry as soon
// as their addresses agreed, and all but one became unreachable by name. They
// are keyed by app now, and each keeps the address of the node serving it. An
// address can no longer be shared in the first place: a node is identified by
// its address and service name, so two of them would be the same node.
func TestSeveralProvidersForOneMimeType(t *testing.T) {
	r := testRegistry(t, officeMimeTypes,
		node{address: "collabora:18500", app: "Collabora", mimeTypes: []string{"text/json"}},
		node{address: "o365:18504", app: "MS365", mimeTypes: []string{"text/json"}},
	)

	got, err := r.FindProviders(context.Background(), "text/json")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Collabora", "MS365"}, names(got))

	byName := map[string]string{}
	for _, p := range got {
		byName[p.Name] = p.Address
	}
	assert.Equal(t, map[string]string{"Collabora": "collabora:18500", "MS365": "o365:18504"}, byName)
}

func TestOneEntryPerAppAcrossNodes(t *testing.T) {
	r := testRegistry(t, officeMimeTypes,
		node{address: "node-a:1000", app: "Collabora", mimeTypes: []string{"text/json"}},
		node{address: "node-b:1000", app: "Collabora", mimeTypes: []string{"text/json"}},
	)

	got, err := r.ListProviders(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"Collabora"}, names(got), "an app served by two nodes is still one app")
	assert.Contains(t, []string{"node-a:1000", "node-b:1000"}, got[0].Address)
}

func TestNodesThatCannotServeAreIgnored(t *testing.T) {
	tests := []struct {
		name string
		n    node
	}{
		{"offline", node{address: "a:1000", app: "Gone", mimeTypes: []string{"text/json"}, state: svcregistry.StateOffline}},
		{"draining", node{address: "b:1000", app: "Gone", mimeTypes: []string{"text/json"}, state: svcregistry.StateDraining}},
		{"not a grpc service", node{address: "c:1000", app: "Gone", mimeTypes: []string{"text/json"}, transport: svcregistry.TransportHTTP}},
		{"no app description", node{address: "d:1000"}},
		{"malformed app description", node{address: "e:1000", raw: "}not json{"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testRegistry(t, officeMimeTypes, tt.n)
			got, err := r.ListProviders(context.Background())
			require.NoError(t, err)
			assert.Empty(t, got)
		})
	}
}

func TestNoAppProviderRunning(t *testing.T) {
	r := testRegistry(t, officeMimeTypes)

	got, err := r.ListProviders(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = r.FindProviders(context.Background(), "text/json")
	assert.Error(t, err, "a configured mime type with nothing to open it is still not found")
}

func TestListSupportedMimeTypes(t *testing.T) {
	r := testRegistry(t, officeMimeTypes,
		node{address: "text:1000", app: "TextEditor", mimeTypes: []string{"text/json", "text/xml"}},
	)

	got, err := r.ListSupportedMimeTypes(context.Background())
	require.NoError(t, err)

	require.Len(t, got, 3)
	assert.Equal(t, "text/json", got[0].MimeType, "configured mime types keep their configured order")
	assert.Equal(t, "JSON File", got[0].Name)
	assert.Equal(t, "TextEditor", got[0].DefaultApplication)
	assert.Equal(t, []string{"TextEditor"}, names(got[0].AppProviders))

	assert.Equal(t, "image/bmp", got[1].MimeType)
	assert.Empty(t, got[1].AppProviders, "a configured mime type nothing can open has no apps")

	assert.Equal(t, "text/xml", got[2].MimeType, "a mime type only a provider knows about is appended")
	assert.Equal(t, []string{"TextEditor"}, names(got[2].AppProviders))
}

func TestDefaultProviderForMimeType(t *testing.T) {
	t.Run("resolves the configured default by name", func(t *testing.T) {
		r := testRegistry(t, officeMimeTypes,
			node{address: "text:1000", app: "TextEditor", mimeTypes: []string{"text/json"}},
		)
		got, err := r.GetDefaultProviderForMimeType(context.Background(), "text/json")
		require.NoError(t, err)
		assert.Equal(t, "TextEditor", got.Name)
	})

	t.Run("a default that is not running is not found", func(t *testing.T) {
		r := testRegistry(t, officeMimeTypes)
		_, err := r.GetDefaultProviderForMimeType(context.Background(), "text/json")
		assert.Error(t, err)
	})

	t.Run("a mime type without a configured default is not found", func(t *testing.T) {
		r := testRegistry(t, officeMimeTypes,
			node{address: "image:1000", app: "ImageViewer", mimeTypes: []string{"image/bmp"}},
		)
		_, err := r.GetDefaultProviderForMimeType(context.Background(), "image/bmp")
		assert.Error(t, err)
	})

	t.Run("the default can be changed at runtime", func(t *testing.T) {
		r := testRegistry(t, officeMimeTypes,
			node{address: "text:1000", app: "TextEditor", mimeTypes: []string{"text/json"}},
			node{address: "other:1001", app: "OtherEditor", mimeTypes: []string{"text/json"}},
		)
		require.NoError(t, r.SetDefaultProviderForMimeType(context.Background(), "text/json",
			&registrypb.ProviderInfo{Name: "OtherEditor"}))

		got, err := r.GetDefaultProviderForMimeType(context.Background(), "text/json")
		require.NoError(t, err)
		assert.Equal(t, "OtherEditor", got.Name)
	})

	t.Run("a default given by address still resolves", func(t *testing.T) {
		r := testRegistry(t, officeMimeTypes,
			node{address: "text:1000", app: "TextEditor", mimeTypes: []string{"text/json"}},
		)
		require.NoError(t, r.SetDefaultProviderForMimeType(context.Background(), "text/json",
			&registrypb.ProviderInfo{Address: "text:1000"}))

		got, err := r.GetDefaultProviderForMimeType(context.Background(), "text/json")
		require.NoError(t, err)
		assert.Equal(t, "TextEditor", got.Name)
	})

	t.Run("a default identifying nothing is rejected", func(t *testing.T) {
		r := testRegistry(t, officeMimeTypes)
		err := r.SetDefaultProviderForMimeType(context.Background(), "text/json", &registrypb.ProviderInfo{})
		assert.Error(t, err)
	})
}
