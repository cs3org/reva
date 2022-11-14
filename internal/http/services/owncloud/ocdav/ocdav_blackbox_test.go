// Copyright 2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
package ocdav_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"

	cs3gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	cs3user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	cs3storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/net"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rhttp/global"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/cs3org/reva/v2/tests/cs3mocks/mocks"
	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/otel/trace"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TODO for now we have to test all of ocdav. when this testsuite is complete we can move
// the handlers to dedicated packages to reduce the amount of complexity to get a test environment up
var _ = Describe("ocdav", func() {
	var (
		handler global.Service
		client  *mocks.GatewayAPIClient
		ctx     context.Context

		userspace *cs3storageprovider.StorageSpace
		user      *cs3user.User

		dataSvr *httptest.Server

		basePath string
	)

	BeforeEach(func() {
		user = &cs3user.User{Id: &cs3user.UserId{OpaqueId: "username"}, Username: "username"}

		dataSvr = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		ctx = ctxpkg.ContextSetUser(context.Background(), user)
		client = &mocks.GatewayAPIClient{}

		var err error
		handler, err = ocdav.NewWith(&ocdav.Config{
			FilesNamespace:  "/users/{{.Username}}",
			WebdavNamespace: "/users/{{.Username}}",
		}, nil, ocdav.NewCS3LS(client), nil, trace.NewNoopTracerProvider(), client)
		Expect(err).ToNot(HaveOccurred())

		userspace = &cs3storageprovider.StorageSpace{
			Opaque: &typesv1beta1.Opaque{
				Map: map[string]*typesv1beta1.OpaqueEntry{
					"path": {
						Decoder: "plain",
						Value:   []byte("/users/username/"),
					},
				},
			},
			Id:   &cs3storageprovider.StorageSpaceId{OpaqueId: storagespace.FormatResourceID(cs3storageprovider.ResourceId{StorageId: "provider-1", SpaceId: "foospace", OpaqueId: "root"})},
			Root: &cs3storageprovider.ResourceId{StorageId: "provider-1", SpaceId: "userspace", OpaqueId: "root"},
			Name: "username",
			RootInfo: &cs3storageprovider.ResourceInfo{
				Name: "username",
				Path: "/users/username",
			},
		}

		client.On("GetUser", mock.Anything, mock.Anything).Return(&cs3user.GetUserResponse{
			Status: status.NewNotFound(ctx, "not found"),
		}, nil)
		client.On("GetUserByClaim", mock.Anything, mock.Anything).Return(&cs3user.GetUserByClaimResponse{
			Status: status.NewNotFound(ctx, "not found"),
		}, nil)

		// for public access
		client.On("Authenticate", mock.Anything, mock.MatchedBy(func(req *cs3gateway.AuthenticateRequest) bool {
			return req.Type == "publicshares" &&
				strings.HasPrefix(req.ClientId, "tokenfor") &&
				strings.HasPrefix(req.ClientSecret, "signature||")
		})).Return(&cs3gateway.AuthenticateResponse{
			Status: status.NewOK(ctx),
			User:   user,
			Token:  "jwt",
		}, nil)
		client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
			return req.Ref.ResourceId.StorageId == utils.PublicStorageProviderID &&
				req.Ref.ResourceId.SpaceId == utils.PublicStorageSpaceID &&
				req.Ref.ResourceId.OpaqueId == "tokenforfile"
		})).Return(&cs3storageprovider.StatResponse{
			Status: status.NewOK(ctx),
			Info: &cs3storageprovider.ResourceInfo{
				Type: cs3storageprovider.ResourceType_RESOURCE_TYPE_FILE,
			},
		}, nil)
		client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
			return req.Ref.ResourceId.StorageId == utils.PublicStorageProviderID &&
				req.Ref.ResourceId.SpaceId == utils.PublicStorageSpaceID &&
				req.Ref.ResourceId.OpaqueId == "tokenforfolder"
		})).Return(&cs3storageprovider.StatResponse{
			Status: status.NewOK(ctx),
			Info: &cs3storageprovider.ResourceInfo{
				Type: cs3storageprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
			},
		}, nil)
	})
	AfterEach(func() {
		dataSvr.Close()
	})

	Describe("NewHandler", func() {
		It("returns a handler", func() {
			Expect(handler).ToNot(BeNil())
		})
	})

	// TODO for every endpoint test the different WebDAV Methods

	// basic metadata
	// PROPFIND
	// MKCOL
	// DELETE

	// basic data
	// PUT
	// GET
	// HEAD

	// move & copy
	// MOVE
	// COPY

	// additional methods
	// PROPPATCH
	// LOCK
	// UNLOCK
	// REPORT
	// POST (Tus)
	// OPTIONS?

	Context("at the very legacy /webdav endpoint", func() {

		BeforeEach(func() {
			// set the webdav endpoint to test
			basePath = "/webdav"

			// path based requests at the /webdav endpoint first look up the storage space
			client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
				p := string(req.Opaque.Map["path"].Value)
				return p == "/" || strings.HasPrefix(p, "/users")
			})).Return(&cs3storageprovider.ListStorageSpacesResponse{
				Status:        status.NewOK(ctx),
				StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
			}, nil)
		})

		Describe("PROPFIND", func() {

			var rr *httptest.ResponseRecorder
			var req *http.Request
			var err error

			BeforeEach(func() {
				// setup the request
				rr = httptest.NewRecorder()
				req, err = http.NewRequest("PROPFIND", basePath, strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

			})
			When("the gateway returns a file list", func() {
				It("returns a multistatus with the file info", func() {

					client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
						return req.Ref.Path == "/users/username"
					})).Return(&cs3storageprovider.StatResponse{
						Status: status.NewOK(ctx),
						Info:   userspace.RootInfo,
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusMultiStatus))
					Expect(rr).To(HaveHTTPBody(Not(BeEmpty())), "Body must not be empty")
					// TODO test listing more thoroughly
				})

			})

			When("the gateway returns not found", func() {
				It("returns a not found status", func() {

					client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
						return req.Ref.Path == "/users/username"
					})).Return(&cs3storageprovider.StatResponse{
						Status: status.NewNotFound(ctx, "resource not found"),
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusMultiStatus))
					Expect(rr).To(HaveHTTPBody("<d:multistatus xmlns:s=\"http://sabredav.org/ns\" xmlns:d=\"DAV:\" xmlns:oc=\"http://owncloud.org/ns\"><d:response><d:href>/webdav</d:href><d:propstat><d:prop><oc:name>username</oc:name><d:resourcetype></d:resourcetype><d:getcontentlength>0</d:getcontentlength><oc:favorite>0</oc:favorite></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat><d:propstat><d:prop><oc:file-parent></oc:file-parent></d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat></d:response></d:multistatus>"), "Body must not be empty")
					// TODO test listing more thoroughly

				})
			})
		})

		Describe("MKCOL", func() {

			var rr *httptest.ResponseRecorder
			var req *http.Request
			var err error

			BeforeEach(func() {
				// setup the request
				rr = httptest.NewRecorder()
				req, err = http.NewRequest("MKCOL", basePath+"/newfolder", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

			})

			When("the gateway returns OK", func() {
				It("returns a created status", func() {

					// MKCOL needs to check if the resource already exists to return the correct status
					client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
						return req.Ref.Path == "/users/username/newfolder"
					})).Return(&cs3storageprovider.StatResponse{
						Status: status.NewNotFound(ctx, "not found"),
					}, nil)

					client.On("CreateContainer", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.CreateContainerRequest) bool {
						return utils.ResourceEqual(req.Ref, &cs3storageprovider.Reference{
							ResourceId: userspace.Root,
							Path:       "./newfolder",
						})
					})).Return(&cs3storageprovider.CreateContainerResponse{
						Status: status.NewOK(ctx),
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
					Expect(rr).To(HaveHTTPBody(BeEmpty()), "Body must be empty")
					// TODO expect fileid and etag header?
				})

			})

			When("the parent does not exist", func() {
				It("returns a conflict status", func() {

					// MKCOL needs to check if the resource already exists to return the correct status
					client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
						return req.Ref.Path == "/users/username/newfolder"
					})).Return(&cs3storageprovider.StatResponse{
						Status: status.NewAborted(ctx, errors.New("parent does not exist"), ""),
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusPreconditionFailed))
					// TODO what message does oc10 return? "error: aborted:" is probably not it
					Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\PreconditionFailed</s:exception><s:message>error: aborted: </s:message></d:error>"), "Body must have a precondition failed sabredav exception")

				})
			})

			When("the resource already exists", func() {
				It("returns a method not allowed status", func() {

					// MKCOL needs to check if the resource already exists to return the correct status
					client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
						return req.Ref.Path == "/users/username/newfolder"
					})).Return(&cs3storageprovider.StatResponse{
						Status: status.NewOK(ctx),
						Info:   &cs3storageprovider.ResourceInfo{},
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusMethodNotAllowed))
					Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\MethodNotAllowed</s:exception><s:message>The resource you tried to create already exists</s:message></d:error>"), "Body must have a method not allowed sabredav exception")

				})
			})
		})

		Describe("DELETE", func() {

			var rr *httptest.ResponseRecorder
			var req *http.Request
			var err error

			BeforeEach(func() {
				// setup the request
				rr = httptest.NewRecorder()
				req, err = http.NewRequest("DELETE", basePath+"/existingfolder", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

			})

			When("the gateway returns OK", func() {
				It("returns a created status", func() {

					client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
						return utils.ResourceEqual(req.Ref, &cs3storageprovider.Reference{
							ResourceId: userspace.Root,
							Path:       "./existingfolder",
						})
					})).Return(&cs3storageprovider.DeleteResponse{
						Status: status.NewOK(ctx),
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusNoContent))
					Expect(rr).To(HaveHTTPBody(BeEmpty()), "Body must be empty")
					// TODO expect fileid and etag header?
				})

			})

			When("the gateway returns not found", func() {
				It("returns a method not found status", func() {

					client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
						return utils.ResourceEqual(req.Ref, &cs3storageprovider.Reference{
							ResourceId: userspace.Root,
							Path:       "./existingfolder",
						})
					})).Return(&cs3storageprovider.DeleteResponse{
						Status: status.NewNotFound(ctx, "not found"),
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
					Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\NotFound</s:exception><s:message>Resource not found</s:message></d:error>"), "Body must have a not found sabredav exception")

				})
			})
		})

		Describe("PUT", func() {

			var rr *httptest.ResponseRecorder
			var req *http.Request
			var err error

			BeforeEach(func() {
				// setup the request
				rr = httptest.NewRecorder()
				req, err = http.NewRequest("PUT", basePath+"/newfile", strings.NewReader("new content"))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set(net.HeaderContentLength, "10")
				req = req.WithContext(ctx)

			})

			When("the gateway returns OK", func() {
				It("returns a created status", func() {

					client.On("InitiateFileUpload", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.InitiateFileUploadRequest) bool {
						return utils.ResourceEqual(req.Ref, &cs3storageprovider.Reference{
							ResourceId: userspace.Root,
							Path:       "./newfile",
						})
					})).Return(&cs3gateway.InitiateFileUploadResponse{
						Status: status.NewOK(ctx),
						Protocols: []*cs3gateway.FileUploadProtocol{
							{
								Protocol:       "simple",
								UploadEndpoint: dataSvr.URL,
							},
						},
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
					Expect(rr).To(HaveHTTPBody(BeEmpty()), "Body must be empty")
					// TODO expect fileid and etag header?
				})
			})

			When("the gateway returns aborted", func() {
				It("returns a precondition failed status", func() {

					client.On("InitiateFileUpload", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.InitiateFileUploadRequest) bool {
						return utils.ResourceEqual(req.Ref, &cs3storageprovider.Reference{
							ResourceId: userspace.Root,
							Path:       "./newfile",
						})
					})).Return(&cs3gateway.InitiateFileUploadResponse{
						Status: status.NewAborted(ctx, errors.New("parent does not exist"), "parent does not exist"),
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusPreconditionFailed))
					// TODO Expect(rr).To(HaveHTTPBody(BeEmpty()), "Body must be a sabredav exception")
				})
			})

			When("the resource already exists", func() {
				It("returns a conflict status", func() {

					client.On("InitiateFileUpload", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.InitiateFileUploadRequest) bool {
						return utils.ResourceEqual(req.Ref, &cs3storageprovider.Reference{
							ResourceId: userspace.Root,
							Path:       "./newfile",
						})
					})).Return(&cs3gateway.InitiateFileUploadResponse{
						Status: status.NewFailedPrecondition(ctx, errors.New("precondition failed"), "precondition failed"),
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusConflict))
					// TODO Expect(rr).To(HaveHTTPBody(BeEmpty()), "Body must be a sabredav exception")
				})
			})

			When("the gateway returns not found", func() {
				It("returns a not found", func() {

					client.On("InitiateFileUpload", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.InitiateFileUploadRequest) bool {
						return utils.ResourceEqual(req.Ref, &cs3storageprovider.Reference{
							ResourceId: userspace.Root,
							Path:       "./newfile",
						})
					})).Return(&cs3gateway.InitiateFileUploadResponse{
						Status: status.NewNotFound(ctx, "not found"),
					}, nil)

					handler.Handler().ServeHTTP(rr, req)
					Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
					// TODO Expect(rr).To(HaveHTTPBody(BeEmpty()), "Body must be a sabredav exception")
				})
			})

		})
	})
	Context("at the /dav/avatars endpoint", func() {

		BeforeEach(func() {
			basePath = "/dav/avatars"
		})

	})
	Context("at the legacy /dav/files endpoint", func() {

		BeforeEach(func() {
			basePath = "/dav/files"
		})

	})
	Context("at the /dav/meta endpoint", func() {

		BeforeEach(func() {
			basePath = "/dav/meta"
		})

	})
	Context("at the /dav/trash-bin endpoint", func() {

		BeforeEach(func() {
			basePath = "/dav/trash-bin"
		})

	})
	Context("at the /dav/spaces endpoint", func() {

		BeforeEach(func() {
			basePath = "/dav/spaces"
		})

	})
	Context("at the /dav/public-files endpoint", func() {

		BeforeEach(func() {
			basePath = "/dav/public-files"
		})

	})

	// TODO restructure the tests and split them up by endpoint?
	// - that should allow reusing the set up of expected requests to the gateway

	// listing spaces is a precondition for path based requests, what if listing spaces currently is broken?
	Context("bad requests", func() {

		It("to the /dav/spaces endpoint root return a method not allowed status ", func() {
			rr := httptest.NewRecorder()
			req, err := http.NewRequest("DELETE", "/dav/spaces", strings.NewReader(""))
			Expect(err).ToNot(HaveOccurred())
			req = req.WithContext(ctx)

			handler.Handler().ServeHTTP(rr, req)
			Expect(rr).To(HaveHTTPStatus(http.StatusMethodNotAllowed))
		})
		It("when deleting a space at the /dav/spaces endpoint return method not allowed status", func() {
			rr := httptest.NewRecorder()
			req, err := http.NewRequest("DELETE", "/dav/spaces/trytodeleteme", strings.NewReader(""))
			Expect(err).ToNot(HaveOccurred())
			req = req.WithContext(ctx)

			handler.Handler().ServeHTTP(rr, req)
			Expect(rr).To(HaveHTTPStatus(http.StatusMethodNotAllowed))
			Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\MethodNotAllowed</s:exception><s:message>deleting spaces via dav is not allowed</s:message></d:error>"), "Body must have a sabredav exception")
		})
		It("with invalid if header return bad request status", func() {
			rr := httptest.NewRecorder()
			req, err := http.NewRequest("DELETE", "/dav/spaces/somespace/foo", strings.NewReader(""))
			req.Header.Set("If", "invalid")
			Expect(err).ToNot(HaveOccurred())
			req = req.WithContext(ctx)

			handler.Handler().ServeHTTP(rr, req)
			Expect(rr).To(HaveHTTPStatus(http.StatusBadRequest))
		})

		DescribeTable("returns 415 when no body was expected",
			func(method string, path string) {
				// as per https://www.rfc-editor.org/rfc/rfc4918#section-8.4
				rr := httptest.NewRecorder()
				req, err := http.NewRequest(method, path, strings.NewReader("should be empty"))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(http.StatusUnsupportedMediaType))
				Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\UnsupportedMediaType</s:exception><s:message>body must be empty</s:message></d:error>"), "Body must have a sabredav exception")
			},
			Entry("MOVE", "MOVE", "/webdav/source"),
			Entry("COPY", "COPY", "/webdav/source"),
			Entry("DELETE", "DELETE", "/webdav/source"),
			PEntry("MKCOL", "MKCOL", "/webdav/source"),
		)

		DescribeTable("check naming rules",
			func(method string, path string, expectedStatus int) {
				rr := httptest.NewRecorder()
				req, err := http.NewRequest(method, "", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req.URL.Path = path // we need to overwrite the path here to send invalid chars

				if method == "COPY" || method == "MOVE" {
					req.Header.Set(net.HeaderDestination, path+".bak")
				}

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))

				Expect(rr).To(HaveHTTPBody(HavePrefix("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\BadRequest</s:exception><s:message>")), "Body must have a sabredav exception")
			},
			Entry("MKCOL no \\f", "MKCOL", "/webdav/forbidden \f char", http.StatusBadRequest),
			Entry("MKCOL no \\r", "MKCOL", "/webdav/forbidden \r char", http.StatusBadRequest),
			Entry("MKCOL no \\n", "MKCOL", "/webdav/forbidden \n char", http.StatusBadRequest),
			Entry("MKCOL no \\\\", "MKCOL", "/webdav/forbidden \\ char", http.StatusBadRequest),

			// COPY source path
			Entry("COPY no \\f", "COPY", "/webdav/forbidden \f char", http.StatusBadRequest),
			Entry("COPY no \\r", "COPY", "/webdav/forbidden \r char", http.StatusBadRequest),
			Entry("COPY no \\n", "COPY", "/webdav/forbidden \n char", http.StatusBadRequest),
			Entry("COPY no \\\\", "COPY", "/webdav/forbidden \\ char", http.StatusBadRequest),

			// MOVE source path
			Entry("MOVE no \\f", "MOVE", "/webdav/forbidden \f char", http.StatusBadRequest),
			Entry("MOVE no \\r", "MOVE", "/webdav/forbidden \r char", http.StatusBadRequest),
			Entry("MOVE no \\n", "MOVE", "/webdav/forbidden \n char", http.StatusBadRequest),
			Entry("MOVE no \\\\", "MOVE", "/webdav/forbidden \\ char", http.StatusBadRequest),
		)

		DescribeTable("check naming rules",
			func(method string, path string, expectedStatus int) {
				rr := httptest.NewRecorder()
				req, err := http.NewRequest(method, "/webdav/safe path", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())

				req.Header.Set(net.HeaderDestination, path+".bak")

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))

				Expect(rr).To(HaveHTTPBody(HavePrefix("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\BadRequest</s:exception><s:message>")), "Body must have a sabredav exception")
			},
			// COPY
			Entry("COPY no \\f", "COPY", "/webdav/forbidden \f char", http.StatusBadRequest),
			Entry("COPY no \\r", "COPY", "/webdav/forbidden \r char", http.StatusBadRequest),
			Entry("COPY no \\n", "COPY", "/webdav/forbidden \n char", http.StatusBadRequest),
			Entry("COPY no \\\\", "COPY", "/webdav/forbidden \\ char", http.StatusBadRequest),

			// MOVE
			Entry("MOVE no \\f", "MOVE", "/webdav/forbidden \f char", http.StatusBadRequest),
			Entry("MOVE no \\r", "MOVE", "/webdav/forbidden \r char", http.StatusBadRequest),
			Entry("MOVE no \\n", "MOVE", "/webdav/forbidden \n char", http.StatusBadRequest),
			Entry("MOVE no \\\\", "MOVE", "/webdav/forbidden \\ char", http.StatusBadRequest),
		)

	})

	// listing spaces is a precondition for path based requests, what if listing spaces currently is broken?
	Context("When listing spaces fails with an error", func() {

		DescribeTable("HandleDelete",
			func(endpoint string, expectedPathPrefix string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(nil, fmt.Errorf("unexpected io error"))

				// the spaces endpoint omits the list storage spaces call, it directly executes the delete call
				client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
					return utils.ResourceEqual(req.Ref, &cs3storageprovider.Reference{
						ResourceId: userspace.Root,
						Path:       "./foo",
					})
				})).Return(&cs3storageprovider.DeleteResponse{
					Status: status.NewOK(ctx),
				}, nil)

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("DELETE", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				if expectedStatus == http.StatusInternalServerError {
					Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception></s:exception><s:message>unexpected io error</s:message></d:error>"), "Body must have a sabredav exception")
				} else {
					Expect(rr).To(HaveHTTPBody(""), "Body must be empty")
				}

			},
			Entry("at the /webdav endpoint", "/webdav", "/users", http.StatusInternalServerError),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", http.StatusInternalServerError),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", http.StatusNoContent),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", http.StatusInternalServerError),
		)

		DescribeTable("HandleMkcol",
			func(endpoint string, expectedPathPrefix string, expectedStatPath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(nil, fmt.Errorf("unexpected io error"))

				// path based requests need to check if the resource already exists
				client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
					return req.Ref.Path == expectedStatPath
				})).Return(&cs3storageprovider.StatResponse{
					Status: status.NewNotFound(ctx, "not found"),
				}, nil)

				// the spaces endpoint omits the list storage spaces call, it directly executes the create container call
				client.On("CreateContainer", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.CreateContainerRequest) bool {
					return utils.ResourceEqual(req.Ref, &cs3storageprovider.Reference{
						ResourceId: userspace.Root,
						Path:       "./foo",
					})
				})).Return(&cs3storageprovider.CreateContainerResponse{
					Status: status.NewOK(ctx),
				}, nil)

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("MKCOL", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				if expectedStatus == http.StatusInternalServerError {
					Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception></s:exception><s:message>unexpected io error</s:message></d:error>"), "Body must have a sabredav exception")
				} else {
					Expect(rr).To(HaveHTTPBody(""), "Body must be empty")
				}

			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "/users/username/foo", http.StatusInternalServerError),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "/users/username/foo", http.StatusInternalServerError),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "/users/username/foo", http.StatusCreated),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "/public/tokenforfolder/foo", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", "/public/tokenforfolder/foo", http.StatusInternalServerError),
		)
	})

	Context("When calls fail with an error", func() {

		DescribeTable("HandleDelete",
			func(endpoint string, expectedPathPrefix string, expectedPath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedPath,
				}

				client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
					return utils.ResourceEqual(req.Ref, &ref)
				})).Return(nil, fmt.Errorf("unexpected io error"))

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("DELETE", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				if expectedStatus == http.StatusInternalServerError {
					Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception></s:exception><s:message>unexpected io error</s:message></d:error>"), "Body must have a sabredav exception")
				} else {
					Expect(rr).To(HaveHTTPBody(""), "Body must be empty")
				}

			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "./foo", http.StatusInternalServerError),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "./foo", http.StatusInternalServerError),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "./foo", http.StatusInternalServerError),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", ".", http.StatusInternalServerError),
		)

		DescribeTable("HandleMkcol",
			func(endpoint string, expectedPathPrefix string, expectedStatPath string, expectedCreatePath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace}, // FIXME we may need to return the /public storage provider id and mock it
				}, nil)

				// path based requests need to check if the resource already exists
				client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
					return req.Ref.Path == expectedStatPath
				})).Return(&cs3storageprovider.StatResponse{
					Status: status.NewNotFound(ctx, "not found"),
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedCreatePath,
				}

				client.On("CreateContainer", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.CreateContainerRequest) bool {
					return utils.ResourceEqual(req.Ref, &ref)
				})).Return(nil, fmt.Errorf("unexpected io error"))

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("MKCOL", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				if expectedStatus == http.StatusInternalServerError {
					Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception></s:exception><s:message>unexpected io error</s:message></d:error>"), "Body must have a sabredav exception")
				} else {
					Expect(rr).To(HaveHTTPBody(""), "Body must be empty")
				}

			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "/users/username/foo", "./foo", http.StatusInternalServerError),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "/users/username/foo", "./foo", http.StatusInternalServerError),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "/users/username/foo", "./foo", http.StatusInternalServerError),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "/public/tokenforfolder/foo", "./foo", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", "/public/tokenforfolder/foo", ".", http.StatusInternalServerError),
		)

	})

	Context("When calls return ok", func() {

		DescribeTable("HandleDelete",
			func(endpoint string, expectedPathPrefix string, expectedPath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedPath,
				}

				client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
					return utils.ResourceEqual(req.Ref, &ref)
				})).Return(&cs3storageprovider.DeleteResponse{
					Status: status.NewOK(ctx),
				}, nil)

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("DELETE", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				Expect(rr).To(HaveHTTPBody(""), "Body must be empty")

			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "./foo", http.StatusNoContent),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "./foo", http.StatusNoContent),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "./foo", http.StatusNoContent),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", ".", http.StatusNoContent),
		)

		DescribeTable("HandleMkcol",
			func(endpoint string, expectedPathPrefix string, expectedStatPath string, expectedCreatePath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace}, // FIXME we may need to return the /public storage provider id and mock it
				}, nil)

				// path based requests need to check if the resource already exists
				client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
					return req.Ref.Path == expectedStatPath
				})).Return(&cs3storageprovider.StatResponse{
					Status: status.NewNotFound(ctx, "not found"),
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedCreatePath,
				}

				client.On("CreateContainer", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.CreateContainerRequest) bool {
					return utils.ResourceEqual(req.Ref, &ref)
				})).Return(&cs3storageprovider.CreateContainerResponse{
					Status: status.NewOK(ctx),
				}, nil)

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("MKCOL", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				Expect(rr).To(HaveHTTPBody(""), "Body must be empty")

			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "/users/username/foo", "./foo", http.StatusCreated),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "/users/username/foo", "./foo", http.StatusCreated),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "/users/username/foo", "./foo", http.StatusCreated),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "/public/tokenforfolder/foo", "./foo", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", "/public/tokenforfolder/foo", ".", http.StatusCreated),
		)

	})

	Context("When the resource is not found", func() {

		DescribeTable("HandleDelete",
			func(endpoint string, expectedPathPrefix string, expectedPath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedPath,
				}

				client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
					return utils.ResourceEqual(req.Ref, &ref)
				})).Return(&cs3storageprovider.DeleteResponse{
					Status: status.NewNotFound(ctx, "not found"),
				}, nil)

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("DELETE", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				if expectedStatus == http.StatusNotFound {
					Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\NotFound</s:exception><s:message>Resource not found</s:message></d:error>"), "Body must have a not found sabredav exception")
				} else {
					Expect(rr).To(HaveHTTPBody(""), "Body must be empty")
				}
			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "./foo", http.StatusNotFound),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "./foo", http.StatusNotFound),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "./foo", http.StatusNotFound),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", ".", http.StatusNotFound),
		)

		DescribeTable("HandleMkcol",
			func(endpoint string, expectedPathPrefix string, expectedStatPath string, expectedPath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
				}, nil)

				// path based requests need to check if the resource already exists
				client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
					return req.Ref.Path == expectedStatPath
				})).Return(&cs3storageprovider.StatResponse{
					Status: status.NewNotFound(ctx, "not found"),
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedPath,
				}

				client.On("CreateContainer", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.CreateContainerRequest) bool {
					return utils.ResourceEqual(req.Ref, &ref)
				})).Return(&cs3storageprovider.CreateContainerResponse{
					Status: status.NewNotFound(ctx, "not found"),
				}, nil)

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("MKCOL", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				if expectedStatus == http.StatusNotFound {
					Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\NotFound</s:exception><s:message>Resource not found</s:message></d:error>"), "Body must have a not found sabredav exception")
				} else {
					Expect(rr).To(HaveHTTPBody(""), "Body must be empty")
				}
			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "/users/username/foo", "./foo", http.StatusNotFound),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "/users/username/foo", "./foo", http.StatusNotFound),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "/users/username/foo", "./foo", http.StatusNotFound),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "/public/tokenforfolder/foo", "", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", "/public/tokenforfolder/foo", ".", http.StatusNotFound),
		)

	})

	Context("When the operation is forbidden", func() {

		DescribeTable("HandleDelete",
			func(endpoint string, expectedPathPrefix string, expectedPath string, locked, userHasAccess bool, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedPath,
				}

				if locked {
					client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
						return utils.ResourceEqual(req.Ref, &ref)
					})).Return(&cs3storageprovider.DeleteResponse{
						Opaque: &typesv1beta1.Opaque{Map: map[string]*typesv1beta1.OpaqueEntry{
							"lockid": {Decoder: "plain", Value: []byte("somelockid")},
						}},
						Status: status.NewPermissionDenied(ctx, fmt.Errorf("permission denied error"), "permission denied message"),
					}, nil)
				} else {
					client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
						return utils.ResourceEqual(req.Ref, &ref)
					})).Return(&cs3storageprovider.DeleteResponse{
						Status: status.NewPermissionDenied(ctx, fmt.Errorf("permission denied error"), "permission denied message"),
					}, nil)
				}

				if userHasAccess {
					client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
						return utils.ResourceEqual(req.Ref, &ref)
					})).Return(&cs3storageprovider.StatResponse{
						Status: status.NewOK(ctx),
						Info: &cs3storageprovider.ResourceInfo{
							Type: cs3storageprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
						},
					}, nil)
				} else {
					client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
						return utils.ResourceEqual(req.Ref, &ref)
					})).Return(&cs3storageprovider.StatResponse{
						Status: status.NewPermissionDenied(ctx, fmt.Errorf("permission denied error"), "permission denied message"),
					}, nil)
				}

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("DELETE", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				if expectedStatus == http.StatusMethodNotAllowed {
					Expect(rr).To(HaveHTTPBody(""), "Body must be empty")
				} else {
					if userHasAccess {
						if locked {
							Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\Locked</s:exception><s:message></s:message></d:error>"), "Body must have a locked sabredav exception")
							Expect(rr).To(HaveHTTPHeaderWithValue("Lock-Token", "<somelockid>"))
						} else {
							Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\Forbidden</s:exception><s:message></s:message></d:error>"), "Body must have a forbidden sabredav exception")
						}
					} else {
						Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\NotFound</s:exception><s:message>Resource not found</s:message></d:error>"), "Body must have a not found sabredav exception")
					}
				}
			},

			// without lock

			// when user has access he should see forbidden status
			Entry("at the /webdav endpoint", "/webdav", "/users", "./foo", false, true, http.StatusForbidden),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "./foo", false, true, http.StatusForbidden),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "./foo", false, true, http.StatusForbidden),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "", false, true, http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", ".", false, true, http.StatusForbidden),
			// when user does not have access he should get not found status
			Entry("at the /webdav endpoint", "/webdav", "/users", "./foo", false, false, http.StatusNotFound),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "./foo", false, false, http.StatusNotFound),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "./foo", false, false, http.StatusNotFound),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "", false, false, http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", ".", false, false, http.StatusNotFound),

			// With lock

			// when user has access he should see locked status
			Entry("at the /webdav endpoint", "/webdav", "/users", "./foo", true, true, http.StatusLocked),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "./foo", true, true, http.StatusLocked),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "./foo", true, true, http.StatusLocked),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "", true, true, http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", ".", true, true, http.StatusLocked),
			// when user does not have access he should get not found status
			Entry("at the /webdav endpoint", "/webdav", "/users", "./foo", true, false, http.StatusNotFound),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "./foo", true, false, http.StatusNotFound),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "./foo", true, false, http.StatusNotFound),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "", true, false, http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", ".", true, false, http.StatusNotFound),
		)

		DescribeTable("HandleMkcol",
			func(endpoint string, expectedPathPrefix string, expectedStatPath string, expectedPath string, locked, userHasAccess bool, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
				}, nil)

				// path based requests need to check if the resource already exists
				client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
					return req.Ref.Path == expectedStatPath
				})).Return(&cs3storageprovider.StatResponse{
					Status: status.NewNotFound(ctx, "not found"),
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedPath,
				}

				if locked {
					client.On("CreateContainer", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.CreateContainerRequest) bool {
						return utils.ResourceEqual(req.Ref, &ref)
					})).Return(&cs3storageprovider.CreateContainerResponse{
						Opaque: &typesv1beta1.Opaque{Map: map[string]*typesv1beta1.OpaqueEntry{
							"lockid": {Decoder: "plain", Value: []byte("somelockid")},
						}},
						Status: status.NewPermissionDenied(ctx, fmt.Errorf("permission denied error"), "permission denied message"),
					}, nil)
				} else {
					client.On("CreateContainer", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.CreateContainerRequest) bool {
						return utils.ResourceEqual(req.Ref, &ref)
					})).Return(&cs3storageprovider.CreateContainerResponse{
						Status: status.NewPermissionDenied(ctx, fmt.Errorf("permission denied error"), "permission denied message"),
					}, nil)
				}

				parentRef := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       utils.MakeRelativePath(path.Dir(expectedPath)),
				}

				if userHasAccess {
					client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
						return utils.ResourceEqual(req.Ref, &parentRef)
					})).Return(&cs3storageprovider.StatResponse{
						Status: status.NewOK(ctx),
						Info: &cs3storageprovider.ResourceInfo{
							Type: cs3storageprovider.ResourceType_RESOURCE_TYPE_CONTAINER,
						},
					}, nil)
				} else {
					client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
						return utils.ResourceEqual(req.Ref, &parentRef)
					})).Return(&cs3storageprovider.StatResponse{
						Status: status.NewPermissionDenied(ctx, fmt.Errorf("permission denied error"), "permission denied message"),
					}, nil)
				}

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("MKCOL", endpoint+"/foo", strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
				if expectedStatus == http.StatusMethodNotAllowed {
					Expect(rr).To(HaveHTTPBody(""), "Body must be empty")
				} else {
					if userHasAccess {
						if locked {
							Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\Locked</s:exception><s:message></s:message></d:error>"), "Body must have a locked sabredav exception")
							Expect(rr).To(HaveHTTPHeaderWithValue("Lock-Token", "<somelockid>"))
						} else {
							Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\Forbidden</s:exception><s:message></s:message></d:error>"), "Body must have a forbidden sabredav exception")
						}
					} else {
						Expect(rr).To(HaveHTTPBody("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<d:error xmlns:d=\"DAV\" xmlns:s=\"http://sabredav.org/ns\"><s:exception>Sabre\\DAV\\Exception\\NotFound</s:exception><s:message>Resource not found</s:message></d:error>"), "Body must have a not found sabredav exception")
					}
				}
			},

			// without lock

			// when user has access he should see forbidden status
			Entry("at the /webdav endpoint", "/webdav", "/users", "/users/username/foo", "./foo", false, true, http.StatusForbidden),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "/users/username/foo", "./foo", false, true, http.StatusForbidden),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "/users/username/foo", "./foo", false, true, http.StatusForbidden),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "/public/tokenforfolder/foo", "", false, true, http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", "/public/tokenforfolder/foo", ".", false, true, http.StatusForbidden),
			// when user does not have access he should get not found status
			Entry("at the /webdav endpoint", "/webdav", "/users", "/users/username/foo", "./foo", false, false, http.StatusNotFound),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "/users/username/foo", "./foo", false, false, http.StatusNotFound),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "/users/username/foo", "./foo", false, false, http.StatusNotFound),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "/public/tokenforfolder/foo", "", false, false, http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", "/public/tokenforfolder/foo", ".", false, false, http.StatusNotFound),

			// With lock

			// when user has access he should see locked status
			// FIXME currently the ocdav mkcol handler is not forwarding a lockid ... but decomposedfs at least cannot create locks for unmapped resources, yet
			PEntry("at the /webdav endpoint", "/webdav", "/users", "/users/username/foo", "./foo", true, true, http.StatusLocked),
			PEntry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "/users/username/foo", "./foo", true, true, http.StatusLocked),
			PEntry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "/users/username/foo", "./foo", true, true, http.StatusLocked),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "/public/tokenforfolder/foo", "", true, true, http.StatusMethodNotAllowed),
			PEntry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", "/public/tokenforfolder/foo", ".", true, true, http.StatusLocked),
			// when user does not have access he should get not found status
			Entry("at the /webdav endpoint", "/webdav", "/users", "/users/username/foo", "./foo", true, false, http.StatusNotFound),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "/users/username/foo", "./foo", true, false, http.StatusNotFound),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "/users/username/foo", "./foo", true, false, http.StatusNotFound),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "/public/tokenforfolder/foo", "", true, false, http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", "/public/tokenforfolder/foo", ".", true, false, http.StatusNotFound),
		)

	})
	// listing spaces is a precondition for path based requests, what if listing spaces currently is broken?
	Context("locks are forwarded", func() {

		DescribeTable("HandleDelete",
			func(endpoint string, expectedPathPrefix string, expectedPath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedPath,
				}

				client.On("Delete", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.DeleteRequest) bool {
					Expect(utils.ReadPlainFromOpaque(req.Opaque, "lockid")).To(Equal("urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2"))
					return utils.ResourceEqual(req.Ref, &ref)
				})).Return(&cs3storageprovider.DeleteResponse{
					Status: status.NewOK(ctx),
				}, nil)

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("DELETE", endpoint+"/foo", strings.NewReader(""))
				req.Header.Set("If", "(<urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2>)")
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "./foo", http.StatusNoContent),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "./foo", http.StatusNoContent),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "./foo", http.StatusNoContent),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", ".", http.StatusNoContent),
		)

		// FIXME currently the ocdav mkcol handler is not forwarding a lockid ... but decomposedfs at least cannot create locks for unmapped resources, yet
		PDescribeTable("HandleMkcol",
			func(endpoint string, expectedPathPrefix string, expectedStatPath string, expectedPath string, expectedStatus int) {

				client.On("ListStorageSpaces", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.ListStorageSpacesRequest) bool {
					p := string(req.Opaque.Map["path"].Value)
					return p == "/" || strings.HasPrefix(p, expectedPathPrefix)
				})).Return(&cs3storageprovider.ListStorageSpacesResponse{
					Status:        status.NewOK(ctx),
					StorageSpaces: []*cs3storageprovider.StorageSpace{userspace},
				}, nil)

				// path based requests need to check if the resource already exists
				client.On("Stat", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.StatRequest) bool {
					return req.Ref.Path == expectedStatPath
				})).Return(&cs3storageprovider.StatResponse{
					Status: status.NewNotFound(ctx, "not found"),
				}, nil)

				ref := cs3storageprovider.Reference{
					ResourceId: userspace.Root,
					Path:       expectedPath,
				}

				client.On("CreateContainer", mock.Anything, mock.MatchedBy(func(req *cs3storageprovider.CreateContainerRequest) bool {
					Expect(utils.ReadPlainFromOpaque(req.Opaque, "lockid")).To(Equal("urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2"))
					return utils.ResourceEqual(req.Ref, &ref)
				})).Return(&cs3storageprovider.CreateContainerResponse{
					Status: status.NewOK(ctx),
				}, nil)

				rr := httptest.NewRecorder()
				req, err := http.NewRequest("MKCOL", endpoint+"/foo", strings.NewReader(""))
				req.Header.Set("If", "(<urn:uuid:181d4fae-7d8c-11d0-a765-00a0c91e6bf2>)")
				Expect(err).ToNot(HaveOccurred())
				req = req.WithContext(ctx)

				handler.Handler().ServeHTTP(rr, req)
				Expect(rr).To(HaveHTTPStatus(expectedStatus))
			},
			Entry("at the /webdav endpoint", "/webdav", "/users", "/users/username/foo", "./foo", http.StatusNoContent),
			Entry("at the /dav/files endpoint", "/dav/files/username", "/users/username", "/users/username/foo", "./foo", http.StatusNoContent),
			Entry("at the /dav/spaces endpoint", "/dav/spaces/provider-1$userspace!root", "/users/username", "/users/username/foo", "./foo", http.StatusNoContent),
			Entry("at the /dav/public-files endpoint for a file", "/dav/public-files/tokenforfile", "", "/public/tokenforfolder/foo", "", http.StatusMethodNotAllowed),
			Entry("at the /dav/public-files endpoint for a folder", "/dav/public-files/tokenforfolder", "/public/tokenforfolder", "/public/tokenforfolder/foo", ".", http.StatusNoContent),
		)

	})

})
