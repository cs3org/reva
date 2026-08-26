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

package sciencemesh

import (
	"errors"
	"testing"

	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func TestSenderOriginFromProtocolsPrefersWebDAV(t *testing.T) {
	t.Parallel()

	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebdavOptions{WebdavOptions: &ocmpb.WebDAVProtocol{
			Uri: "https://dav.sender.example/remote.php/dav/ocm/shares/abc",
		}}},
		{Term: &ocmpb.Protocol_WebappOptions{WebappOptions: &ocmpb.WebappProtocol{
			Uri: "https://app.sender.example/s/abc",
		}}},
	}

	origin, err := senderOriginFromProtocols(protocols)
	if err != nil {
		t.Fatalf("senderOriginFromProtocols() error = %v", err)
	}
	if origin != "https://dav.sender.example" {
		t.Fatalf("origin = %q, want https://dav.sender.example", origin)
	}
}

func TestSenderOriginFromProtocolsWebappFallback(t *testing.T) {
	t.Parallel()

	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebappOptions{WebappOptions: &ocmpb.WebappProtocol{
			Uri: "https://app.sender.example/s/abc",
		}}},
	}

	origin, err := senderOriginFromProtocols(protocols)
	if err != nil {
		t.Fatalf("senderOriginFromProtocols() error = %v", err)
	}
	if origin != "https://app.sender.example" {
		t.Fatalf("origin = %q, want https://app.sender.example", origin)
	}
}

func TestSenderOriginFromProtocolsMissing(t *testing.T) {
	t.Parallel()

	_, err := senderOriginFromProtocols(nil)
	if err == nil {
		t.Fatal("expected error for missing protocols")
	}
	var notFound errtypes.NotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("expected NotFound, got %T: %v", err, err)
	}
}
