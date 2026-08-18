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

package grpc_test

import (
	"context"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storagep "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/service"
	jwt "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/metadata"
)

// This suite tests the service-registry data-path discovery (PR #5665, §8-§11
// of the design): a storage provider must find its paired data provider through
// the registry, by mount_id affinity, with no data_server_url configured.
//
// The fixture runs a storage provider (gRPC) and a data provider (HTTP) in the
// same revad. They share one in-process memory registry; the data provider
// self-registers with mount_id=discovery-mount-id and an explicit public_url.
// On InitiateFileUpload/Download the storage provider resolves the data server
// purely from the registry, so the returned endpoint must be built from that
// public_url.
var _ = Describe("service registry data-path discovery", func() {
	var (
		dependencies = map[string]string{}
		variables    = map[string]string{}
		revads       = map[string]*Revad{}

		ctx           context.Context
		serviceClient storagep.ProviderAPIClient
		user          = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "einstein",
		}

		fileRef = &storagep.Reference{Path: "/file"}

		// the public_url the data provider advertises in the fixture, with the
		// {{http_address}} placeholder resolved to the revad's http port.
		expectedDataServer string
	)

	BeforeEach(func() {
		dependencies = map[string]string{
			"storage": "storageprovider-registry-discovery.toml",
		}
	})

	JustBeforeEach(func() {
		var err error
		ctx = context.Background()

		tokenManager, err := jwt.New(map[string]any{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		s, err := scope.AddOwnerScope(nil)
		Expect(err).ToNot(HaveOccurred())
		tkn, err := tokenManager.MintToken(ctx, user, s)
		Expect(err).ToNot(HaveOccurred())
		ctx = appctx.ContextSetToken(ctx, tkn)
		ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, tkn)
		ctx = appctx.ContextSetUser(ctx, user)

		revads, err = startRevads(dependencies, nil, nil, variables)
		Expect(err).ToNot(HaveOccurred())
		Expect(revads["storage"]).ToNot(BeNil())

		expectedDataServer = "http://" + revads["storage"].HTTPAddress + "/data"

		serviceClient, err = service.StorageProviderAt(revads["storage"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed)).To(Succeed())
		}
	})

	It("resolves the upload endpoint from the registry by mount_id", func() {
		res, err := serviceClient.InitiateFileUpload(ctx, &storagep.InitiateFileUploadRequest{Ref: fileRef})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
		Expect(res.Protocols).ToNot(BeEmpty())

		for _, p := range res.Protocols {
			// The endpoint is <data-server public_url>/<protocol>/<id>, so it must
			// start with the URL the data provider advertised in the registry.
			Expect(p.UploadEndpoint).To(HavePrefix(expectedDataServer),
				"upload endpoint %q should be derived from the registry-advertised data server %q",
				p.UploadEndpoint, expectedDataServer)
			Expect(p.Expose).To(BeTrue())
		}
	})

	It("resolves the download endpoint from the registry by mount_id", func() {
		// Seed a file so the download can be initiated.
		_, err := serviceClient.InitiateFileUpload(ctx, &storagep.InitiateFileUploadRequest{Ref: fileRef})
		Expect(err).ToNot(HaveOccurred())

		res, err := serviceClient.InitiateFileDownload(ctx, &storagep.InitiateFileDownloadRequest{Ref: fileRef})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Status.Code).To(Equal(rpc.Code_CODE_OK))
		Expect(res.Protocols).ToNot(BeEmpty())

		for _, p := range res.Protocols {
			Expect(p.DownloadEndpoint).To(HavePrefix(expectedDataServer),
				"download endpoint %q should be derived from the registry-advertised data server %q",
				p.DownloadEndpoint, expectedDataServer)
			// sanity: the path is preserved after the data server base.
			Expect(strings.TrimPrefix(p.DownloadEndpoint, expectedDataServer)).ToNot(BeEmpty())
		}
	})
})
