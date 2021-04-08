package filecache_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestFilecache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filecache Suite")
}
