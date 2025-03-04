//go:build linux
// +build linux

// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package posix_test

import (
	"os"

	"github.com/owncloud/reva/v2/pkg/storage/fs/posix"
	"github.com/owncloud/reva/v2/tests/helpers"
	"github.com/rs/zerolog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Posix", func() {
	var (
		options map[string]interface{}
		tmpRoot string
	)

	BeforeEach(func() {
		tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		options = map[string]interface{}{
			"root":           tmpRoot,
			"share_folder":   "/Shares",
			"permissionssvc": "any",
			"idcache": map[string]interface{}{
				"cache_store": "nats-js-kv",
			},
		}
	})

	AfterEach(func() {
		if tmpRoot != "" {
			os.RemoveAll(tmpRoot)
		}
	})

	Describe("New", func() {
		It("returns a new instance", func() {
			_, err := posix.New(options, nil, &zerolog.Logger{})
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
