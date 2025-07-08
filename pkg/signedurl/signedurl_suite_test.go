package signedurl_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSignedurl(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Signedurl Suite")
}
