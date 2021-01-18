package s3ng_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cs3org/reva/pkg/storage/fs/s3ng"
)

var _ = Describe("S3ng", func() {
	Describe("NewDefault", func() {
		It("fails on missing s3 configuration", func() {
			_, err := s3ng.NewDefault(map[string]interface{}{})
			Expect(err).To(MatchError("S3 configuration incomplete"))
		})
	})
})
