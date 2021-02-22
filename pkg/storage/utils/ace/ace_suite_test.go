package ace_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAce(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ace Suite")
}
