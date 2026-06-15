package kiteworks_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKiteworks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kiteworks driver")
}
