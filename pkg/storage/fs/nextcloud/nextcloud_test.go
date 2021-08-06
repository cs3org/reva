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

package nextcloud_test

import (
	"context"
	"net/http"
	"os"

	"github.com/cs3org/reva/pkg/storage/fs/nextcloud"
	"github.com/cs3org/reva/tests/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Nextcloud", func() {
	var (
		options map[string]interface{}
		tmpRoot string
	)

	BeforeEach(func() {
		tmpRoot, err := helpers.TempDir("reva-unit-tests-*-root")
		Expect(err).ToNot(HaveOccurred())

		options = map[string]interface{}{
			"root":         tmpRoot,
			"enable_home":  true,
			"share_folder": "/Shares",
		}
	})

	AfterEach(func() {
		if tmpRoot != "" {
			os.RemoveAll(tmpRoot)
		}
	})

	Describe("New", func() {
		It("returns a new instance", func() {
			_, err := nextcloud.New(options)
			Expect(err).ToNot(HaveOccurred())
		})
	})
	Describe("CreateHome", func() {
		It("calls the CreateHome endpoint", func() {
			nc, _ := nextcloud.NewStorageDriver(&nextcloud.StorageDriverConfig{})

			const (
				okResponse = `{
					"users": [
						{"id": 1, "name": "Roman"},
						{"id": 2, "name": "Dmitry"}
					]	
				}`
			)
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := w.Write([]byte(okResponse))
				if err != nil {
					panic(err)
				}
			})
			mock, teardown := helpers.TestingHTTPClient(h)
			defer teardown()
			nc.SetHTTPClient(mock)
			err2 := nc.CreateHome(context.TODO())
			Expect(err2).ToNot(HaveOccurred())
		})
	})
})
