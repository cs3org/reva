package propfind_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPropfind(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Propfind Suite")
}
