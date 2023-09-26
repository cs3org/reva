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

package groups_test

import (
	"context"
	"database/sql"
	"os"

	_ "github.com/mattn/go-sqlite3"

	"github.com/cs3org/reva/v2/pkg/group/manager/owncloudsql/groups"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Groups", func() {
	var (
		conn       *groups.Groups
		testDbFile *os.File
		sqldb      *sql.DB
	)

	BeforeEach(func() {
		var err error
		testDbFile, err = os.CreateTemp("", "example")
		Expect(err).ToNot(HaveOccurred())

		dbData, err := os.ReadFile("test.sqlite")
		Expect(err).ToNot(HaveOccurred())

		_, err = testDbFile.Write(dbData)
		Expect(err).ToNot(HaveOccurred())
		err = testDbFile.Close()
		Expect(err).ToNot(HaveOccurred())

		sqldb, err = sql.Open("sqlite3", testDbFile.Name())
		Expect(err).ToNot(HaveOccurred())

	})

	AfterEach(func() {
		os.Remove(testDbFile.Name())
	})

	Describe("GetAccountByClaim", func() {

		Context("without any joins", func() {

			BeforeEach(func() {
				var err error
				conn, err = groups.New("sqlite3", sqldb, false, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("gets existing group by group_id", func() {
				groupID := "admin"
				group, err := conn.GetGroupByClaim(context.Background(), "group_id", groupID)
				Expect(err).ToNot(HaveOccurred())
				Expect(group).ToNot(BeNil())
				Expect(group.GID).To(Equal("admin"))
			})

			It("gets existing group by group_name", func() {
				groupName := "admin"
				group, err := conn.GetGroupByClaim(context.Background(), "group_name", groupName)
				Expect(err).ToNot(HaveOccurred())
				Expect(group).ToNot(BeNil())
				Expect(group.GID).To(Equal("admin"))
			})

		})

	})

})
