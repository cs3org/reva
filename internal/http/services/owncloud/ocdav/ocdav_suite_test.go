package ocdav_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestOcdav(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ocdav Suite")
}
