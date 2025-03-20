package jsoncs3_test

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	apppb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/opencloud-eu/reva/v2/pkg/appauth"
	"github.com/opencloud-eu/reva/v2/pkg/appauth/manager/jsoncs3"
	"github.com/opencloud-eu/reva/v2/pkg/auth/scope"
	ctxpkg "github.com/opencloud-eu/reva/v2/pkg/ctx"
	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/metadata"
	mdMock "github.com/opencloud-eu/reva/v2/pkg/storage/utils/metadata/mocks"
	"github.com/opencloud-eu/reva/v2/pkg/utils"
)

const (
	testUsername = "Testuser"
	testUserID   = "test-user-id"
	testUserIDP  = "test-user-idp"
)

var _ = Describe("Jsoncs3", func() {
	Describe("New", func() {
		var (
			config = map[string]any{
				"provider_addr":       "eu.opencloud.api.storage-system",
				"service_user_id":     "service-user-id",
				"service_user_idp":    "service-user-idp",
				"machine_auth_apikey": "machineauthpw",
			}
		)
		It("Works with a valid config", func() {
			m, err := jsoncs3.New(config)
			Expect(err).ToNot(HaveOccurred())
			Expect(m).ToNot(BeNil())
		})
		It("Fails with an incomplete config", func() {
			m, err := jsoncs3.New(map[string]any{})
			Expect(err).To(HaveOccurred())
			Expect(m).To(BeNil())
		})
	})
	Describe("When configured", func() {
		var (
			ctx     context.Context
			manager appauth.Manager
			md      *mdMock.Storage
			scopes  map[string]*authpb.Scope
			err     error
		)

		BeforeEach(func() {
			var gen jsoncs3.PasswordGenerator
			gen, err = jsoncs3.NewDicewareGenerator(map[string]any{
				"number_of_words": 3,
			})
			Expect(err).ToNot(HaveOccurred())

			md = mdMock.NewStorage(GinkgoT())
			md.EXPECT().Init(mock.Anything, "jsoncs3-appauth-data").Return(nil).Once()
			manager, err = jsoncs3.NewWithOptions(md, gen)
			Expect(err).ToNot(HaveOccurred())
			Expect(manager).ToNot(BeNil())

			scopes, err = scope.AddOwnerScope(map[string]*authpb.Scope{})
			Expect(err).ToNot(HaveOccurred())

			ctx = ctxpkg.ContextSetUser(
				context.Background(),
				&user.User{
					Username: testUsername,
					Id: &user.UserId{
						OpaqueId: testUserID,
						Idp:      testUserIDP,
					},
				})
		})
		Describe("Initialization", func() {
			It("is only done once on the first request", func() {
				md.EXPECT().Download(mock.Anything, mock.Anything).Return(nil, errtypes.NotFound("unit test"))
				md.EXPECT().Upload(mock.Anything, mock.Anything).Return(nil, nil)
				_, err = manager.GenerateAppPassword(ctx, scopes, "testing", nil)
				Expect(err).ToNot(HaveOccurred())

				// subsequent calls don't re-initialize the storage
				_, err = manager.GenerateAppPassword(ctx, scopes, "testing", nil)
				Expect(err).ToNot(HaveOccurred())
			})
		})
		Describe("GenerateAppPassword", func() {
			When("The user does not have any app passwords", func() {
				BeforeEach(func() {
					md.EXPECT().Download(mock.Anything, mock.Anything).Return(nil, errtypes.NotFound("unit test")).Once()
				})
				It("adds a new password and returns a valid response", func() {
					md.On("Upload",
						mock.Anything,
						mock.MatchedBy(func(req metadata.UploadRequest) bool {
							return req.Path == testUserID+".json"
						}),
					).Return(nil, nil)
					apppw, err := manager.GenerateAppPassword(ctx, scopes, "testing", nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(apppw.Password).ToNot(BeEmpty())
					Expect(len(strings.Split(apppw.Password, " "))).To(Equal(3))
				})
			})
			When("The user already has app passwords", func() {
				var (
					existingAppPw = map[string]*apppb.AppPassword{
						"existing-id": {
							Password: "hash",
						},
					}
					content []byte
				)
				var uploadedPw map[string]*apppb.AppPassword
				BeforeEach(func() {
					uploadedPw = map[string]*apppb.AppPassword{}
					content, err = json.Marshal(existingAppPw)
					Expect(err).ToNot(HaveOccurred())
					dlRes := metadata.DownloadResponse{
						Content: content,
						Etag:    "1",
					}
					md.EXPECT().Download(mock.Anything, mock.Anything).Return(&dlRes, nil).Once()
				})
				It("adds a new app password and returns a valid response", func() {
					md.EXPECT().Upload(
						mock.Anything,
						mock.MatchedBy(func(req metadata.UploadRequest) bool {
							if req.Path != testUserID+".json" {
								return false
							}
							err := json.Unmarshal(req.Content, &uploadedPw)
							return err == nil
						}),
					).Return(nil, nil).Once()
					apppw, err := manager.GenerateAppPassword(ctx, scopes, "testing", nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(uploadedPw)).To(Equal(2))
					_, ok := uploadedPw["existing-id"]
					Expect(ok).To(BeTrue())
					Expect(apppw.Password).ToNot(BeEmpty())
					Expect(len(strings.Split(apppw.Password, " "))).To(Equal(3))
				})
				It("retries when the etag on the app password changed", func() {
					dlRes := metadata.DownloadResponse{
						Content: content,
						Etag:    "2",
					}
					md.EXPECT().Download(mock.Anything, mock.Anything).Return(&dlRes, nil).Once()
					md.EXPECT().Upload(
						mock.Anything,
						mock.MatchedBy(func(req metadata.UploadRequest) bool {
							if req.Path != testUserID+".json" {
								return false
							}
							if req.IfMatchEtag != "1" {
								return false
							}
							err := json.Unmarshal(req.Content, &uploadedPw)
							return err == nil
						}),
					).Return(nil, errtypes.PreconditionFailed("etag mismatch")).Once()
					md.EXPECT().Upload(
						mock.Anything,
						mock.MatchedBy(func(req metadata.UploadRequest) bool {
							if req.Path != testUserID+".json" {
								return false
							}
							if req.IfMatchEtag != "2" {
								return false
							}
							err := json.Unmarshal(req.Content, &uploadedPw)
							return err == nil
						}),
					).Return(nil, nil).Once()
					apppw, err := manager.GenerateAppPassword(ctx, scopes, "testing", nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(uploadedPw)).To(Equal(2))
					_, ok := uploadedPw["existing-id"]
					Expect(ok).To(BeTrue())
					Expect(apppw.Password).ToNot(BeEmpty())
					Expect(len(strings.Split(apppw.Password, " "))).To(Equal(3))
				})
				It("retries 5 times at maximum when the etag changes", func() {
					dlRes := metadata.DownloadResponse{
						Content: content,
						Etag:    "1",
					}
					md.EXPECT().Download(mock.Anything, mock.Anything).Return(&dlRes, nil).Times(4)
					md.EXPECT().Upload(
						mock.Anything,
						mock.MatchedBy(func(req metadata.UploadRequest) bool {
							if req.Path != testUserID+".json" {
								return false
							}
							if req.IfMatchEtag != "1" {
								return false
							}
							err := json.Unmarshal(req.Content, &uploadedPw)
							return err == nil
						}),
					).Return(nil, errtypes.PreconditionFailed("etag mismatch")).Times(5)
					apppw, err := manager.GenerateAppPassword(ctx, scopes, "testing", nil)
					Expect(err).To(HaveOccurred())
					Expect(len(uploadedPw)).To(Equal(2))
					_, ok := uploadedPw["existing-id"]
					Expect(ok).To(BeTrue())
					Expect(apppw).To(BeNil())
				})
			})
		})
		Describe("GetAppPassword", func() {
			var (
				existingAppPw = map[string]*apppb.AppPassword{}
				uploadedPw    = map[string]*apppb.AppPassword{}
				content       []byte
				dlRes         *metadata.DownloadResponse
			)
			BeforeEach(func() {
				hash, err := argon2id.CreateHash("password", argon2id.DefaultParams)
				Expect(err).ToNot(HaveOccurred())
				existingAppPw["existing-id"] = &apppb.AppPassword{
					Password: hash,
					Utime:    utils.TSNow(),
				}

				content, err = json.Marshal(existingAppPw)
				Expect(err).ToNot(HaveOccurred())
				dlRes = &metadata.DownloadResponse{
					Content: content,
				}

				md.EXPECT().Download(mock.Anything, mock.Anything).Return(dlRes, nil)
			})
			It("Succeeds when the password matches", func() {
				_, err = manager.GetAppPassword(ctx, &user.UserId{OpaqueId: "userid"}, "password")
				Expect(err).ToNot(HaveOccurred())
			})
			It("Succeeds fails when the password does not match", func() {
				_, err = manager.GetAppPassword(ctx, &user.UserId{OpaqueId: "userid"}, "wrong password")
				Expect(err).Should(MatchError(errtypes.NotFound("password not found")))
			})
			It("Only uploads when the last utime update was more that 5 minute ago", func() {
				utime := time.Now().Add(-6 * time.Minute)
				existingAppPw["existing-id"].Utime = utils.TimeToTS(utime)
				content, err = json.Marshal(existingAppPw)
				dlRes.Content = content
				md.EXPECT().Upload(
					mock.Anything,
					mock.MatchedBy(func(req metadata.UploadRequest) bool {
						err := json.Unmarshal(req.Content, &uploadedPw)
						return err == nil
					}),
				).Return(nil, nil)
				pw, err := manager.GetAppPassword(ctx, &user.UserId{OpaqueId: "userid"}, "password")
				updatedUTime := utils.TSToTime(pw.Utime)
				Expect(updatedUTime.Sub(utime)).To(BeNumerically(">", 5*time.Minute))
				Expect(err).ToNot(HaveOccurred())
			})
		})
		Describe("ListAppPasswords", func() {
			var existingAppPw = map[string]*apppb.AppPassword{
				"existing-id": {
					Password: "hash",
				},
			}
			BeforeEach(func() {
				content, err := json.Marshal(existingAppPw)
				Expect(err).ToNot(HaveOccurred())
				dlRes := metadata.DownloadResponse{
					Content: content,
				}

				md.EXPECT().Download(mock.Anything, mock.Anything).Return(&dlRes, nil)
			})
			It("Returns the user's apppassword", func() {
				res, err := manager.ListAppPasswords(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(res)).To(Equal(1))

			})
		})
		Describe("InvalidateAppPassword", func() {
			var appPasswords = map[string]*apppb.AppPassword{}
			var hash2 string
			BeforeEach(func() {
				hash1, err := argon2id.CreateHash("password1", argon2id.DefaultParams)
				Expect(err).ToNot(HaveOccurred())
				hash2, err = argon2id.CreateHash("password2", argon2id.DefaultParams)
				Expect(err).ToNot(HaveOccurred())
				appPasswords["id1"] = &apppb.AppPassword{
					Password: hash1,
				}
				appPasswords["id2"] = &apppb.AppPassword{
					Password: hash2,
				}
				content, err := json.Marshal(appPasswords)
				Expect(err).ToNot(HaveOccurred())
				dlRes := metadata.DownloadResponse{
					Content: content,
				}
				md.EXPECT().Download(mock.Anything, mock.Anything).Return(&dlRes, nil)
			})

			It("Returns an error when the password does not match any password in the store", func() {
				err := manager.InvalidateAppPassword(ctx, "test")
				Expect(err).To(MatchError(errtypes.NotFound("password not found")))
			})
			It("Removes the requested password from the store", func() {
				var uploadedPw map[string]*apppb.AppPassword
				md.EXPECT().Upload(
					mock.Anything,
					mock.MatchedBy(func(req metadata.UploadRequest) bool {
						if req.Path != testUserID+".json" {
							return false
						}
						err := json.Unmarshal(req.Content, &uploadedPw)
						return err == nil
					}),
				).Return(nil, nil)
				err := manager.InvalidateAppPassword(ctx, "password2")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadedPw)).To(Equal(1))

				_, ok := uploadedPw["id1"]
				Expect(ok).To(BeTrue())

				_, ok = uploadedPw["id2"]
				Expect(ok).To(BeFalse())
			})
			It("Removes the requested password from the store when using the password hash", func() {
				var uploadedPw map[string]*apppb.AppPassword
				md.EXPECT().Upload(
					mock.Anything,
					mock.MatchedBy(func(req metadata.UploadRequest) bool {
						if req.Path != testUserID+".json" {
							return false
						}
						err := json.Unmarshal(req.Content, &uploadedPw)
						return err == nil
					}),
				).Return(nil, nil)
				err := manager.InvalidateAppPassword(ctx, hash2)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(uploadedPw)).To(Equal(1))

				_, ok := uploadedPw["id1"]
				Expect(ok).To(BeTrue())

				_, ok = uploadedPw["id2"]
				Expect(ok).To(BeFalse())
			})
		})
	})
})
