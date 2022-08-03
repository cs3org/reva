package providercache_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProvidercache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Providercache Suite")
}
