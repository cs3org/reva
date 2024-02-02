package mtimesyncedcache_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMtimesyncedcache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mtimesyncedcache Suite")
}
