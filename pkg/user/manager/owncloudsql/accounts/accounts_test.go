// Copyright 2018-2023 CERN
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

package accounts_test

import (
	"context"
	"database/sql"
	"os"

	"github.com/cs3org/reva/pkg/user/manager/owncloudsql/accounts"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Accounts", func() {
	var (
		conn       *accounts.Accounts
		testDBFile *os.File
		sqldb      *sql.DB
	)

	BeforeEach(func() {
		var err error
		testDBFile, err = os.CreateTemp("", "example")
		Expect(err).ToNot(HaveOccurred())

		dbData, err := os.ReadFile("test.sqlite")
		Expect(err).ToNot(HaveOccurred())

		_, err = testDBFile.Write(dbData)
		Expect(err).ToNot(HaveOccurred())
		err = testDBFile.Close()
		Expect(err).ToNot(HaveOccurred())

		sqldb, err = sql.Open("sqlite3", testDBFile.Name())
		Expect(err).ToNot(HaveOccurred())

	})

	AfterEach(func() {
		os.Remove(testDBFile.Name())
	})

	Describe("GetAccountByClaim", func() {

		Context("without any joins", func() {

			BeforeEach(func() {
				var err error
				conn, err = accounts.New("sqlite3", sqldb, false, false, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("gets existing account by userid", func() {
				userID := "admin"
				account, err := conn.GetAccountByClaim(context.Background(), "userid", userID)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("admin"))
				Expect(account.OwnCloudUUID.String).To(Equal("admin"))
			})

			It("gets existing account by mail", func() {
				value := "admin@example.org"
				account, err := conn.GetAccountByClaim(context.Background(), "mail", value)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("admin"))
				Expect(account.OwnCloudUUID.String).To(Equal("admin"))
			})

			It("falls back to user_id colum when getting by username", func() {
				value := "admin"
				account, err := conn.GetAccountByClaim(context.Background(), "username", value)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("admin"))
				Expect(account.OwnCloudUUID.String).To(Equal("admin"))
			})

			It("errors on unsupported claim", func() {
				_, err := conn.GetAccountByClaim(context.Background(), "invalid", "invalid")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with username joins", func() {

			BeforeEach(func() {
				var err error
				conn, err = accounts.New("sqlite3", sqldb, true, false, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("gets existing account by userid", func() {
				userID := "admin"
				account, err := conn.GetAccountByClaim(context.Background(), "userid", userID)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("Administrator"))
				Expect(account.OwnCloudUUID.String).To(Equal("admin"))
			})

			It("gets existing account by mail", func() {
				value := "admin@example.org"
				account, err := conn.GetAccountByClaim(context.Background(), "mail", value)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("Administrator"))
				Expect(account.OwnCloudUUID.String).To(Equal("admin"))
			})

			It("gets existing account by username", func() {
				value := "Administrator"
				account, err := conn.GetAccountByClaim(context.Background(), "username", value)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("Administrator"))
				Expect(account.OwnCloudUUID.String).To(Equal("admin"))
			})

			It("errors on unsupported claim", func() {
				_, err := conn.GetAccountByClaim(context.Background(), "invalid", "invalid")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with uuid joins", func() {

			BeforeEach(func() {
				var err error
				conn, err = accounts.New("sqlite3", sqldb, false, true, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("gets existing account by uuid", func() {
				userID := "7015b5ec-7723-4560-bb96-85e18a947314"
				account, err := conn.GetAccountByClaim(context.Background(), "userid", userID)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("admin"))
				Expect(account.OwnCloudUUID.String).To(Equal("7015b5ec-7723-4560-bb96-85e18a947314"))
			})

			It("gets existing account by mail", func() {
				value := "admin@example.org"
				account, err := conn.GetAccountByClaim(context.Background(), "mail", value)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("admin"))
				Expect(account.OwnCloudUUID.String).To(Equal("7015b5ec-7723-4560-bb96-85e18a947314"))
			})

			It("gets existing account by username", func() {
				value := "admin"
				account, err := conn.GetAccountByClaim(context.Background(), "username", value)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("admin"))
				Expect(account.OwnCloudUUID.String).To(Equal("7015b5ec-7723-4560-bb96-85e18a947314"))
			})

			It("errors on unsupported claim", func() {
				_, err := conn.GetAccountByClaim(context.Background(), "invalid", "invalid")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with username and uuid joins", func() {

			BeforeEach(func() {
				var err error
				conn, err = accounts.New("sqlite3", sqldb, true, true, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("gets existing account by uuid", func() {
				userID := "7015b5ec-7723-4560-bb96-85e18a947314"
				account, err := conn.GetAccountByClaim(context.Background(), "userid", userID)
				Expect(err).ToNot(HaveOccurred())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("Administrator"))
				Expect(account.OwnCloudUUID.String).To(Equal("7015b5ec-7723-4560-bb96-85e18a947314"))
			})

			It("gets existing account by mail", func() {
				value := "admin@example.org"
				account, err := conn.GetAccountByClaim(context.Background(), "mail", value)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("Administrator"))
				Expect(account.OwnCloudUUID.String).To(Equal("7015b5ec-7723-4560-bb96-85e18a947314"))
			})

			It("gets existing account by username", func() {
				value := "Administrator"
				account, err := conn.GetAccountByClaim(context.Background(), "username", value)
				Expect(err).ToNot(HaveOccurred())
				Expect(account).ToNot(BeNil())
				Expect(account.ID).To(Equal(uint64(1)))
				Expect(account.Email.String).To(Equal("admin@example.org"))
				Expect(account.UserID).To(Equal("admin"))
				Expect(account.DisplayName.String).To(Equal("admin"))
				Expect(account.Quota.String).To(Equal("100 GB"))
				Expect(account.LastLogin).To(Equal(1619082575))
				Expect(account.Backend).To(Equal(`OC\User\Database`))
				Expect(account.Home).To(Equal("/mnt/data/files/admin"))
				Expect(account.State).To(Equal(int8(1)))
				Expect(account.Username.String).To(Equal("Administrator"))
				Expect(account.OwnCloudUUID.String).To(Equal("7015b5ec-7723-4560-bb96-85e18a947314"))
			})

			It("errors on unsupported claim", func() {
				_, err := conn.GetAccountByClaim(context.Background(), "invalid", "invalid")
				Expect(err).To(HaveOccurred())
			})
		})

	})

	Describe("FindAccounts", func() {

		Context("with username and uuid joins", func() {

			BeforeEach(func() {
				var err error
				conn, err = accounts.New("sqlite3", sqldb, true, true, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("finds the existing admin account", func() {
				accounts, err := conn.FindAccounts(context.Background(), "admin")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(accounts)).To(Equal(1))
				Expect(accounts[0]).ToNot(BeNil())
				Expect(accounts[0].ID).To(Equal(uint64(1)))
				Expect(accounts[0].Email.String).To(Equal("admin@example.org"))
				Expect(accounts[0].UserID).To(Equal("admin"))
				Expect(accounts[0].DisplayName.String).To(Equal("admin"))
				Expect(accounts[0].Quota.String).To(Equal("100 GB"))
				Expect(accounts[0].LastLogin).To(Equal(1619082575))
				Expect(accounts[0].Backend).To(Equal(`OC\User\Database`))
				Expect(accounts[0].Home).To(Equal("/mnt/data/files/admin"))
				Expect(accounts[0].State).To(Equal(int8(1)))
				Expect(accounts[0].Username.String).To(Equal("Administrator"))
				Expect(accounts[0].OwnCloudUUID.String).To(Equal("7015b5ec-7723-4560-bb96-85e18a947314"))
			})

			It("handles query without results", func() {
				accounts, err := conn.FindAccounts(context.Background(), "__notexisting__")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(accounts)).To(Equal(0))
			})
		})

		Context("with username joins", func() {

			BeforeEach(func() {
				var err error
				conn, err = accounts.New("sqlite3", sqldb, true, false, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("finds the existing admin account", func() {
				accounts, err := conn.FindAccounts(context.Background(), "admin")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(accounts)).To(Equal(1))
				Expect(accounts[0]).ToNot(BeNil())
				Expect(accounts[0].ID).To(Equal(uint64(1)))
				Expect(accounts[0].Email.String).To(Equal("admin@example.org"))
				Expect(accounts[0].UserID).To(Equal("admin"))
				Expect(accounts[0].DisplayName.String).To(Equal("admin"))
				Expect(accounts[0].Quota.String).To(Equal("100 GB"))
				Expect(accounts[0].LastLogin).To(Equal(1619082575))
				Expect(accounts[0].Backend).To(Equal(`OC\User\Database`))
				Expect(accounts[0].Home).To(Equal("/mnt/data/files/admin"))
				Expect(accounts[0].State).To(Equal(int8(1)))
				Expect(accounts[0].Username.String).To(Equal("Administrator"))
				Expect(accounts[0].OwnCloudUUID.String).To(Equal("admin"))
			})

			It("handles query without results", func() {
				accounts, err := conn.FindAccounts(context.Background(), "__notexisting__")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(accounts)).To(Equal(0))
			})
		})

		Context("without any joins", func() {

			BeforeEach(func() {
				var err error
				conn, err = accounts.New("sqlite3", sqldb, false, false, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("finds the existing admin account", func() {
				accounts, err := conn.FindAccounts(context.Background(), "admin")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(accounts)).To(Equal(1))
				Expect(accounts[0]).ToNot(BeNil())
				Expect(accounts[0].ID).To(Equal(uint64(1)))
				Expect(accounts[0].Email.String).To(Equal("admin@example.org"))
				Expect(accounts[0].UserID).To(Equal("admin"))
				Expect(accounts[0].DisplayName.String).To(Equal("admin"))
				Expect(accounts[0].Quota.String).To(Equal("100 GB"))
				Expect(accounts[0].LastLogin).To(Equal(1619082575))
				Expect(accounts[0].Backend).To(Equal(`OC\User\Database`))
				Expect(accounts[0].Home).To(Equal("/mnt/data/files/admin"))
				Expect(accounts[0].State).To(Equal(int8(1)))
				Expect(accounts[0].Username.String).To(Equal("admin"))
				Expect(accounts[0].OwnCloudUUID.String).To(Equal("admin"))
			})

			It("handles query without results", func() {
				accounts, err := conn.FindAccounts(context.Background(), "__notexisting__")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(accounts)).To(Equal(0))
			})
		})
	})

	Describe("GetAccountGroups", func() {
		BeforeEach(func() {
			var err error
			conn, err = accounts.New("sqlite3", sqldb, true, true, false)
			Expect(err).ToNot(HaveOccurred())
		})
		It("get admin group for admin account", func() {
			accounts, err := conn.GetAccountGroups(context.Background(), "admin")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(accounts)).To(Equal(1))
			Expect(accounts[0]).To(Equal("admin"))
		})
		It("handles not existing account", func() {
			accounts, err := conn.GetAccountGroups(context.Background(), "__notexisting__")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(accounts)).To(Equal(0))
		})
	})
})
