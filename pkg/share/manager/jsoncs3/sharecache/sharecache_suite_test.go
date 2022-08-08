package sharecache_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSharecache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sharecache Suite")
}
