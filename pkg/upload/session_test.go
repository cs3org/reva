package upload

import (
	"context"
	"crypto/sha1" //nolint:gosec
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog"

	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/utils"
)

var _ = Describe("FileSession", func() {
	var (
		ctx  context.Context
		fs   *FileStore
		sess *FileSession
	)

	// newTestStore returns a store rooted in a fresh temp dir, Setup() left to the specs.
	newTestStore := func(opts TokenOptions) *FileStore {
		log := zerolog.Nop()
		return NewFileStore(GinkgoT().TempDir(), opts, &log)
	}

	BeforeEach(func() {
		ctx = context.Background()
		fs = newTestStore(TokenOptions{})
		sess = fs.New(ctx).(*FileSession)
	})

	Describe("typed setters", func() {
		It("stores metadata readable through the typed getter", func() {
			sess.SetMetadata("providerID", "storage-1")
			Expect(sess.ProviderID()).To(Equal("storage-1"))
		})

		It("stores storage values readable through the typed getter", func() {
			sess.SetStorageValue("SpaceRoot", "space-abc")
			Expect(sess.SpaceID()).To(Equal("space-abc"))
		})

		// posix chowns the blob it writes to the space group.
		It("stores the space gid", func() {
			Expect(sess.SpaceGid()).To(BeEmpty())

			sess.SetStorageValue("SpaceGid", "1000")
			Expect(sess.SpaceGid()).To(Equal("1000"))
		})

		// The sync clients use it to recognise their own changes.
		It("stores the initiator id", func() {
			Expect(sess.InitiatorID()).To(BeEmpty())

			sess.SetMetadata("initiatorid", "client-1")
			Expect(sess.InitiatorID()).To(Equal("client-1"))
		})

		It("stores the declared size", func() {
			sess.SetSize(1234)
			Expect(sess.Size()).To(Equal(int64(1234)))
		})

		It("toggles the deferred size flag", func() {
			sess.SetSizeIsDeferred(true)
			Expect(sess.info.SizeIsDeferred).To(BeTrue())
			sess.SetSizeIsDeferred(false)
			Expect(sess.info.SizeIsDeferred).To(BeFalse())
		})
	})

	Describe("SetExecutant", func() {
		It("round-trips the executant id through Executant", func() {
			sess.SetExecutant(&userpb.User{
				Id: &userpb.UserId{
					Idp:      "idp.example.com",
					OpaqueId: "user-42",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Username:    "alice",
				DisplayName: "Alice",
			})

			got := sess.Executant()
			Expect(got.Idp).To(Equal("idp.example.com"))
			Expect(got.OpaqueId).To(Equal("user-42"))
			Expect(got.Type).To(Equal(userpb.UserType_USER_TYPE_PRIMARY))
		})
	})

	Describe("NodeExists", func() {
		It("is false when the key is absent", func() {
			Expect(sess.NodeExists()).To(BeFalse(), "absent key should return false")
		})

		It("is true only for the literal \"true\"", func() {
			sess.SetStorageValue("NodeExists", "true")
			Expect(sess.NodeExists()).To(BeTrue())

			sess.SetStorageValue("NodeExists", "false")
			Expect(sess.NodeExists()).To(BeFalse())

			sess.SetStorageValue("NodeExists", "yes")
			Expect(sess.NodeExists()).To(BeFalse(), "non-'true' value should return false")
		})
	})

	Describe("Checksums", func() {
		It("round-trips the raw bytes SetChecksums stored", func() {
			sha1bytes := []byte{0x01, 0x02, 0x03, 0x04}
			md5bytes := []byte{0x05, 0x06, 0x07, 0x08}
			adlerBytes := []byte{0x09, 0x0a, 0x0b, 0x0c}

			sess.SetChecksums(sha1bytes, md5bytes, adlerBytes)

			got := sess.Checksums()
			Expect(got.SHA1).To(Equal(sha1bytes))
			Expect(got.MD5).To(Equal(md5bytes))
			Expect(got.Adler32).To(Equal(adlerBytes))
		})
	})

	Describe("ScanData", func() {
		It("is empty on a fresh session", func() {
			result, date := sess.ScanData()
			Expect(result).To(BeEmpty(), "fresh session should have no scan result")
			Expect(date.IsZero()).To(BeTrue(), "fresh session should have zero scan date")
		})

		It("round-trips what SetScanData stored", func() {
			now := time.Now().Truncate(time.Second)
			sess.SetScanData("clean", now)

			result, date := sess.ScanData()
			Expect(result).To(Equal("clean"))
			Expect(date).To(BeTemporally("~", now, time.Second))
		})
	})

	Describe("Expires", func() {
		It("is the zero time when unset", func() {
			Expect(sess.Expires().IsZero()).To(BeTrue(), "absent expires should be zero time")
		})

		It("parses the stored OCM mtime", func() {
			want := time.Now().Add(time.Hour).Truncate(time.Second)
			sess.SetMetadata("expires", utils.TimeToOCMtime(want))
			Expect(sess.Expires()).To(BeTemporally("~", want, time.Second))
		})
	})

	Describe("IsProcessing", func() {
		It("is true only once all bytes arrived and no scan result is in", func() {
			sess.SetSize(100)
			Expect(sess.IsProcessing()).To(BeFalse(), "size != offset should not be processing")

			sess.info.Offset = 100
			Expect(sess.IsProcessing()).To(BeTrue(), "size == offset with no scan result should be processing")

			sess.SetScanData("clean", time.Now())
			Expect(sess.IsProcessing()).To(BeFalse(), "scan result set means processing finished")
		})
	})

	Describe("Reference", func() {
		It("assembles the resource id from the stored parts", func() {
			sess.SetMetadata("providerID", "prov-1")
			sess.SetStorageValue("SpaceRoot", "space-2")
			sess.SetStorageValue("NodeId", "node-3")

			ref := sess.Reference()
			Expect(ref.ResourceId).ToNot(BeNil())
			Expect(ref.ResourceId.StorageId).To(Equal("prov-1"))
			Expect(ref.ResourceId.SpaceId).To(Equal("space-2"))
			Expect(ref.ResourceId.OpaqueId).To(Equal("node-3"))
		})
	})

	Describe("Metadata", func() {
		It("carries everything the driver needs at commit time", func() {
			sess.SetMetadata("providerID", "p1")
			sess.SetMetadata("mtime", "12345.0")
			sess.SetStorageValue("NodeExists", "true")
			sess.SetMetadata("if-match", "etag-1")
			sess.SetMetadata("if-none-match", "*")
			sess.SetMetadata("if-unmodified-since", "2026-07-30T10:00:00Z")

			m := sess.Metadata()
			Expect(m["providerID"]).To(Equal("p1"))
			Expect(m["mtime"]).To(Equal("12345.0"))
			Expect(m["nodeExists"]).To(Equal("true"))
			Expect(m["sessionID"]).To(Equal(sess.ID()))
			// The driver re-checks these at PrepareUpload, so they must survive the session.
			Expect(m["if-match"]).To(Equal("etag-1"))
			Expect(m["if-none-match"]).To(Equal("*"))
			Expect(m["if-unmodified-since"]).To(Equal("2026-07-30T10:00:00Z"))
		})
	})

	Describe("Persist", func() {
		It("survives a round trip through the store", func() {
			Expect(fs.Setup()).To(Succeed())

			sess.SetSize(512)
			sess.SetMetadata("providerID", "prov-rt")
			sess.SetStorageValue("NodeId", "nd-rt")
			sess.SetStorageValue("NodeExists", "true")

			Expect(sess.TouchBin()).To(Succeed())
			Expect(sess.Persist(ctx)).To(Succeed())

			loaded, err := fs.Get(ctx, sess.ID())
			Expect(err).ToNot(HaveOccurred())
			ls := loaded.(*FileSession)

			Expect(ls.Size()).To(Equal(int64(512)))
			Expect(ls.ProviderID()).To(Equal("prov-rt"))
			Expect(ls.NodeID()).To(Equal("nd-rt"))
			Expect(ls.NodeExists()).To(BeTrue())
		})

		It("creates intermediate directories", func() {
			log := zerolog.Nop()
			nested := NewFileStore(filepath.Join(GinkgoT().TempDir(), "sub", "nested"), TokenOptions{}, &log)
			s := nested.New(ctx).(*FileSession)
			s.SetSize(1)

			Expect(s.Persist(ctx)).To(Succeed())
			_, err := os.Stat(s.infoPath())
			Expect(err).ToNot(HaveOccurred())
		})

		// The coordinator undoes the upload on this failure, so it cannot just be logged.
		It("reports an upload directory it cannot create", func() {
			blocked := filepath.Join(GinkgoT().TempDir(), "file")
			Expect(os.WriteFile(blocked, nil, 0600)).To(Succeed())
			log := zerolog.Nop()
			s := NewFileStore(filepath.Join(blocked, "uploads"), TokenOptions{}, &log).New(ctx).(*FileSession)

			Expect(s.Persist(ctx)).ToNot(Succeed())
		})
	})

	Describe("WriteChunk", func() {
		It("appends and advances the offset", func() {
			Expect(os.MkdirAll(filepath.Dir(sess.binPath()), 0700)).To(Succeed())
			Expect(sess.TouchBin()).To(Succeed())

			n, err := sess.WriteChunk(ctx, 0, strings.NewReader("hello"))
			Expect(err).ToNot(HaveOccurred())
			Expect(n).To(Equal(int64(5)))
			Expect(sess.Offset()).To(Equal(int64(5)))

			n2, err := sess.WriteChunk(ctx, 5, strings.NewReader(" world"))
			Expect(err).ToNot(HaveOccurred())
			Expect(n2).To(Equal(int64(6)))
			Expect(sess.Offset()).To(Equal(int64(11)))

			data, err := os.ReadFile(sess.binPath())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(Equal("hello world"))
		})

		It("fails when the staging file does not exist", func() {
			Expect(os.MkdirAll(filepath.Dir(sess.binPath()), 0700)).To(Succeed())

			_, err := sess.WriteChunk(ctx, 0, strings.NewReader("data"))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Cleanup", func() {
		// A persisted session with both files on disk.
		var staged *FileSession

		BeforeEach(func() {
			staged = sess
			Expect(os.MkdirAll(filepath.Dir(staged.binPath()), 0700)).To(Succeed())
			Expect(staged.TouchBin()).To(Succeed())
			Expect(staged.Persist(ctx)).To(Succeed())
		})

		It("removes the binary only", func() {
			staged.Cleanup(ctx, true, false)
			_, err := os.Stat(staged.binPath())
			Expect(os.IsNotExist(err)).To(BeTrue(), "bin should be removed")
			_, err = os.Stat(staged.infoPath())
			Expect(err).ToNot(HaveOccurred(), "info should survive")
		})

		It("removes the info only", func() {
			staged.Cleanup(ctx, false, true)
			_, err := os.Stat(staged.binPath())
			Expect(err).ToNot(HaveOccurred(), "bin should survive")
			_, err = os.Stat(staged.infoPath())
			Expect(os.IsNotExist(err)).To(BeTrue(), "info should be removed")
		})

		It("removes both", func() {
			staged.Cleanup(ctx, true, true)
			_, err := os.Stat(staged.binPath())
			Expect(os.IsNotExist(err)).To(BeTrue(), "bin should be removed")
			_, err = os.Stat(staged.infoPath())
			Expect(os.IsNotExist(err)).To(BeTrue(), "info should be removed")
		})

		It("removes nothing when both flags are false", func() {
			staged.Cleanup(ctx, false, false)
			_, err := os.Stat(staged.binPath())
			Expect(err).ToNot(HaveOccurred(), "bin should survive")
			_, err = os.Stat(staged.infoPath())
			Expect(err).ToNot(HaveOccurred(), "info should survive")
		})

		It("tolerates files that are already gone", func() {
			fresh := fs.New(ctx).(*FileSession)
			Expect(func() { fresh.Cleanup(ctx, true, true) }).ToNot(Panic())
		})

		// The caller is already tearing the upload down, so this is only worth logging.
		It("reports nothing when a file cannot be removed", func() {
			// A non-empty directory in place of each file fails removal.
			for _, path := range []string{staged.binPath(), staged.infoPath()} {
				Expect(os.Remove(path)).To(Succeed())
				Expect(os.MkdirAll(filepath.Join(path, "in-the-way"), 0700)).To(Succeed())
			}

			Expect(func() { staged.Cleanup(ctx, true, true) }).ToNot(Panic())

			_, err := os.Stat(staged.binPath())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Purge", func() {
		It("removes both files", func() {
			Expect(sess.TouchBin()).To(Succeed())
			Expect(sess.Persist(ctx)).To(Succeed())

			sess.Purge(ctx)

			_, err := os.Stat(sess.binPath())
			Expect(os.IsNotExist(err)).To(BeTrue(), "bin should be removed")
			_, err = os.Stat(sess.infoPath())
			Expect(os.IsNotExist(err)).To(BeTrue(), "info should be removed")
		})
	})

	// tusd answers a HEAD from the raw FileInfo, not from the getters.
	Describe("ToFileInfo", func() {
		It("hands out the info the session holds", func() {
			sess.SetSize(512)
			sess.SetMetadata("providerID", "prov-1")

			info := sess.ToFileInfo()

			Expect(info.ID).To(Equal(sess.ID()))
			Expect(info.Size).To(Equal(int64(512)))
			Expect(info.MetaData["providerID"]).To(Equal("prov-1"))
		})
	})

	Describe("Context", func() {
		It("restores the user, lock id, and initiator id", func() {
			sess.SetExecutant(&userpb.User{
				Id: &userpb.UserId{
					Idp:      "idp.test",
					OpaqueId: "ctx-user",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				},
				Username: "ctxuser",
			})
			sess.SetMetadata("lockid", "lock-xyz")
			sess.SetMetadata("initiatorid", "initiator-abc")

			sessCtx := sess.Context(ctx)

			gotUser, ok := ctxpkg.ContextGetUser(sessCtx)
			Expect(ok).To(BeTrue())
			Expect(gotUser.GetId().GetOpaqueId()).To(Equal("ctx-user"))
			Expect(gotUser.GetId().GetIdp()).To(Equal("idp.test"))

			lockID, ok := ctxpkg.ContextGetLockID(sessCtx)
			Expect(ok).To(BeTrue())
			Expect(lockID).To(Equal("lock-xyz"))

			initiator, ok := ctxpkg.ContextGetInitiator(sessCtx)
			Expect(ok).To(BeTrue())
			Expect(initiator).To(Equal("initiator-abc"))
		})
	})

	Describe("URL", func() {
		It("signs a transfer token behind the data gateway endpoint", func() {
			store := newTestStore(TokenOptions{
				DownloadEndpoint:     "http://download.example.com",
				DataGatewayEndpoint:  "http://gateway.example.com",
				TransferSharedSecret: "s3cr3t",
				TransferExpires:      3600,
			})
			s := store.New(ctx).(*FileSession)

			url, err := s.URL(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(url).ToNot(BeEmpty())
			Expect(url).To(HavePrefix("http://gateway.example.com"), "URL should start with DataGatewayEndpoint")
		})

		It("can be called repeatedly", func() {
			store := newTestStore(TokenOptions{
				DataGatewayEndpoint:  "http://gw.example.com",
				TransferSharedSecret: "secret",
				TransferExpires:      60,
			})
			s := store.New(ctx).(*FileSession)

			url1, err := s.URL(ctx)
			Expect(err).ToNot(HaveOccurred())
			url2, err := s.URL(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(url1).ToNot(BeEmpty())
			Expect(url2).ToNot(BeEmpty())
		})
	})
})

var _ = Describe("calculateChecksums", func() {
	It("computes sha1, md5, and adler32 in one pass", func() {
		path := filepath.Join(GinkgoT().TempDir(), "testfile")
		Expect(os.WriteFile(path, []byte("hello"), 0600)).To(Succeed())

		sha1h, md5h, adler32h, err := calculateChecksums(context.Background(), path)
		Expect(err).ToNot(HaveOccurred())

		Expect(hex.EncodeToString(sha1h.Sum(nil))).To(Equal("aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"))
		Expect(hex.EncodeToString(md5h.Sum(nil))).To(Equal("5d41402abc4b2a76b9719d911017c592"))
		Expect(hex.EncodeToString(adler32h.Sum(nil))).To(Equal("062c0215"))
	})

	It("errors on a missing file", func() {
		_, _, _, err := calculateChecksums(context.Background(), "/nonexistent/path/file.bin")
		Expect(err).To(HaveOccurred())
	})

	// A file that opens but cannot be read to the end, such as a vanished mount.
	It("errors when the file cannot be read through", func() {
		_, _, _, err := calculateChecksums(context.Background(), GinkgoT().TempDir())
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("checkHash", func() {
	It("accepts a matching digest", func() {
		hash := sha1.New() //nolint:gosec
		hash.Write([]byte("hello"))
		Expect(checkHash("aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d", hash)).To(Succeed())
	})

	It("reports a mismatch as errtypes.ChecksumMismatch", func() {
		hash := sha1.New() //nolint:gosec
		hash.Write([]byte("hello"))

		err := checkHash("0000000000000000000000000000000000000000", hash)
		Expect(err).To(HaveOccurred())
		Expect(errors.As(err, new(errtypes.ChecksumMismatch))).To(BeTrue())
	})
})

var _ = DescribeTable("joinURLParts",
	func(parts []string, want string) {
		Expect(joinURLParts(parts...)).To(Equal(want))
	},
	Entry("trailing slash on the base", []string{"http://host/", "path"}, "http://host/path"),
	Entry("no trailing slash on the base", []string{"http://host", "path"}, "http://host/path"),
	Entry("single part", []string{"http://host"}, "http://host"),
	Entry("three parts", []string{"http://host", "a", "b"}, "http://host/a/b"),
	Entry("slashes already present", []string{"http://host/", "a/", "b"}, "http://host/a/b"),
)
