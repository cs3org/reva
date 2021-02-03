package s3ng_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestS3ng(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "S3ng Suite")
}
