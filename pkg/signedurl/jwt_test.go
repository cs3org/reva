package signedurl_test

import (
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opencloud-eu/reva/v2/pkg/signedurl"
)

var _ = Describe("JWTSignedURL", func() {
	It("should create a new JWTSignedURL with a valid key", func() {
		key := "my-secret-key"
		jwtURL, err := signedurl.NewJWTSignedURL(signedurl.WithSecret(key), signedurl.WithQueryParam("sig"))
		Expect(err).ToNot(HaveOccurred())
		Expect(jwtURL).ToNot(BeNil())
	})

	It("should return an error when creating JWTSignedURL with an empty key", func() {
		jwtURL, err := signedurl.NewJWTSignedURL(signedurl.WithQueryParam("sig"))
		Expect(err).To(HaveOccurred())
		Expect(jwtURL).To(BeNil())
		Expect(err).To(Equal(signedurl.ErrInvalidKey))
	})
	Context("with a valid JWTSignedURL", func() {
		var jwtURL *signedurl.JWTSignedURL
		var unsignedURL string

		BeforeEach(func() {
			var err error
			jwtURL, err = signedurl.NewJWTSignedURL(signedurl.WithSecret("my-secret-key"), signedurl.WithQueryParam("sig"))
			Expect(err).ToNot(HaveOccurred())
			unsignedURL = "https://example.com/resource"
		})

		It("should return a signed URL", func() {
			signedURL, err := jwtURL.Sign(unsignedURL, "", 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())
			url, err := url.Parse(signedURL)
			Expect(err).ToNot(HaveOccurred())
			query := url.Query()
			sig := query.Get("sig")
			Expect(sig).ToNot(BeEmpty())
			query.Del("sig")
			url.RawQuery = query.Encode()
			Expect(url.String()).To(Equal(unsignedURL))
		})
		It("should return an error when signing with an invalid URL", func() {
			invalidURL := "ht tp:not-a-valid-url"
			signedURL, err := jwtURL.Sign(invalidURL, "", 10*time.Minute)
			Expect(err).To(HaveOccurred())
			Expect(signedURL).To(BeEmpty())
			Expect(err).To(MatchError(ContainSubstring("failed to parse url")))
		})
	})
	Context("verifying a signed URL", func() {
		var jwtURL *signedurl.JWTSignedURL
		var unsignedURL string

		BeforeEach(func() {
			var err error
			jwtURL, err = signedurl.NewJWTSignedURL(signedurl.WithSecret("my-secret-key"), signedurl.WithQueryParam("sig"))
			Expect(err).ToNot(HaveOccurred())
		})
		It("fails if the signature is missing", func() {
			unsignedURL = "https://example.com/resource"
			_, err := jwtURL.Verify(unsignedURL)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(signedurl.SignatureVerificationError{}))
		})
		It("fails if the urls to not match", func() {
			signedURL, err := jwtURL.Sign(unsignedURL, "", 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())
			u, err := url.Parse(signedURL)
			Expect(err).ToNot(HaveOccurred())
			q := u.Query()
			q.Set("other", "value")
			u.RawQuery = q.Encode()
			_, err = jwtURL.Verify(u.String())
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(signedurl.SignatureVerificationError{}))
			Expect(err).To(MatchError(ContainSubstring("url mismatch")))
		})
		It("fails if the jwt is expired", func() {
			signedURL, err := jwtURL.Sign(unsignedURL, "", 1*time.Second)
			Expect(err).ToNot(HaveOccurred())
			time.Sleep(3 * time.Second) // wait for the JWT to expire
			_, err = jwtURL.Verify(signedURL)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(signedurl.SignatureVerificationError{}))
			Expect(err).To(MatchError(ContainSubstring("token is expired")))
		})
		It("succeeds if the signature is ok", func() {
			signedURL, err := jwtURL.Sign(unsignedURL, "", 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())
			_, err = jwtURL.Verify(signedURL)
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns the subject of the JWT", func() {
			subject := "subject-id"
			signedURL, err := jwtURL.Sign(unsignedURL, subject, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())
			s, err := jwtURL.Verify(signedURL)
			Expect(err).ToNot(HaveOccurred())
			Expect(s).To(Equal(subject))
		})

	})
})
