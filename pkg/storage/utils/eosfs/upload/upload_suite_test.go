package upload_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUpload(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upload Suite")
}
