package receivedsharecache_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestReceivedShareCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ReceivedShareCache Suite")
}
