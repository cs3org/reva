package eosfs_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEosfs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Eosfs Suite")
}
