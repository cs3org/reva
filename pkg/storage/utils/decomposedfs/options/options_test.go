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

package options_test

import (
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/options"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Options", func() {
	var (
		o      *options.Options
		config map[string]interface{}
	)

	BeforeEach(func() {
		config = map[string]interface{}{}
	})

	Describe("New", func() {
		JustBeforeEach(func() {
			var err error
			o, err = options.New(config)
			Expect(err).ToNot(HaveOccurred())
		})

		It("sets defaults", func() {
			Expect(len(o.ShareFolder) > 0).To(BeTrue())
			Expect(len(o.UserLayout) > 0).To(BeTrue())
		})

		Context("with unclean root path configuration", func() {
			BeforeEach(func() {
				config["root"] = "foo/"
			})

			It("sanitizes the root path", func() {
				Expect(o.Root).To(Equal("foo"))
			})
		})
	})
})
