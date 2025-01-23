package migrator_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMigrator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Migrator Suite")
}
