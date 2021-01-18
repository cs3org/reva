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

package s3ng_test

import (
	"github.com/mitchellh/mapstructure"

	"github.com/cs3org/reva/pkg/storage/fs/s3ng"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Options", func() {
	var (
		o   *s3ng.Options
		raw map[string]interface{}
	)

	BeforeEach(func() {
		raw := map[string]interface{}{
			"s3.endpoint":   "http://1.2.3.4:5000",
			"s3.region":     "default",
			"s3.bucket":     "the-bucket",
			"s3.access_key": "foo",
			"s3.secret_key": "bar",
		}
		o = &s3ng.Options{}
		err := mapstructure.Decode(raw, o)
		Expect(err).ToNot(HaveOccurred())
	})

	It("parses s3 configuration", func() {
		Expect(o.S3Endpoint).To(Equal("http://1.2.3.4:5000"))
		Expect(o.S3Region).To(Equal("default"))
		Expect(o.S3AccessKey).To(Equal("foo"))
		Expect(o.S3SecretKey).To(Equal("bar"))
	})

	Describe("S3ConfigComplete", func() {
		It("returns true", func() {
			Expect(o.S3ConfigComplete()).To(BeTrue())
		})

		It("returns false", func() {
			fields := []string{"s3.endpoint", "s3.region", "s3.bucket", "s3.access_key", "s3.secret_key"}
			for _, f := range fields {
				delete(raw, f)
				o = &s3ng.Options{}
				err := mapstructure.Decode(raw, o)
				Expect(err).ToNot(HaveOccurred())

				Expect(o.S3ConfigComplete()).To(BeFalse(), "failed to return false on missing '%s' field", f)
			}
		})
	})
})
