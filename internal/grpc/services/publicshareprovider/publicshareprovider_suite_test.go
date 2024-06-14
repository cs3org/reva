package publicshareprovider_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPublicShareProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PublicShareProvider Suite")
}
