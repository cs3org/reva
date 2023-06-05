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

package sql_test

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/cs3org/reva/pkg/notification"
	sqlmanager "github.com/cs3org/reva/pkg/notification/manager/sql"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SQL manager for notifications", func() {
	var (
		db         *sql.DB
		testDBFile *os.File
		mgr        notification.Manager
		n1         = &notification.Notification{
			Ref:          "notification-test",
			TemplateName: "notification-template-test",
			Recipients:   []string{"jdoe", "testuser"},
		}
		n2 = &notification.Notification{
			Ref:          "new-notification",
			TemplateName: "new-template",
			Recipients:   []string{"newuser1", "newuser2"},
		}
		nn                              *notification.Notification
		ref                             string
		err                             error
		selectNotificationsSQL          = "SELECT ref, template_name FROM cbox_notifications WHERE ref = ?"
		selectNotificationRecipientsSQL = "SELECT COUNT(*) FROM cbox_notification_recipients WHERE notification_id = ?"
	)

	AfterEach(func() {
		os.Remove(testDBFile.Name())
	})

	BeforeEach(func() {
		var err error
		ref = "notification-test"

		testDBFile, err = os.CreateTemp("", "testdbfile")
		Expect(err).ToNot(HaveOccurred())

		dbData, err := os.ReadFile("test.sqlite")
		Expect(err).ToNot(HaveOccurred())

		_, err = testDBFile.Write(dbData)
		Expect(err).ToNot(HaveOccurred())

		err = testDBFile.Close()
		Expect(err).ToNot(HaveOccurred())

		db, err = sql.Open("sqlite3", fmt.Sprintf("%v?_foreign_keys=on", testDBFile.Name()))
		Expect(err).ToNot(HaveOccurred())
		Expect(db).ToNot(BeNil())

		mgr, err = sqlmanager.New("sqlite3", db)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.Remove(testDBFile.Name())
	})

	Context("Creating notifications", func() {
		When("creating a non-existing notification", func() {
			JustBeforeEach(func() {
				err = mgr.UpsertNotification(*n2)
			})

			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should create a notification entry", func() {
				var newRef, newTemplateName string
				err = db.QueryRow(selectNotificationsSQL, n2.Ref).Scan(&newRef, &newTemplateName)
				Expect(newRef).To(Equal(n2.Ref))
				Expect(newTemplateName).To(Equal(n2.TemplateName))
			})

			It("should create notification recipients entries", func() {
				var notificationID int
				err = db.QueryRow("SELECT id FROM cbox_notifications WHERE ref = ?", n2.Ref).Scan(&notificationID)
				Expect(err).ToNot(HaveOccurred())
				var newRecipientCount int
				err = db.QueryRow(selectNotificationRecipientsSQL, notificationID).Scan(&newRecipientCount)
				Expect(err).ToNot(HaveOccurred())
				Expect(newRecipientCount).To(Equal(len(n2.Recipients)))
			})
		})

		When("updating an existing notification", func() {
			var m = &notification.Notification{
				Ref:          "notification-test",
				TemplateName: "new-notification-template-test",
				Recipients:   []string{"jdoe", "testuser2", "thirduser"},
			}

			JustBeforeEach(func() {
				err = mgr.UpsertNotification(*m)
			})

			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should not increase the number of entries in the notification table", func() {
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM cbox_notifications").Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(1))
			})

			It("should update the existing notification data", func() {
				var newRef, newTemplateName string
				err = db.QueryRow(selectNotificationsSQL, m.Ref).Scan(&newRef, &newTemplateName)
				Expect(newRef).To(Equal(m.Ref))
				Expect(newTemplateName).To(Equal(m.TemplateName))
			})

			It("should delete old entries in notification recipients", func() {
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM cbox_notification_recipients WHERE recipient = 'testuser'").Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(BeZero())
			})

			It("should create new entries in notification recipients", func() {
				var notificationID int
				err = db.QueryRow("SELECT id FROM cbox_notifications WHERE ref = ?", m.Ref).Scan(&notificationID)
				Expect(err).ToNot(HaveOccurred())
				var newRecipientCount int
				err = db.QueryRow(selectNotificationRecipientsSQL, notificationID).Scan(&newRecipientCount)
				Expect(err).ToNot(HaveOccurred())
				Expect(newRecipientCount).To(Equal(len(m.Recipients)))
			})
		})

		When("creating an invalid notification", func() {
			o := &notification.Notification{}

			JustBeforeEach(func() {
				err = mgr.UpsertNotification(*o)
			})

			It("should return an InvalidNotificationError", func() {
				_, isInvalidNotificationError := err.(*notification.InvalidNotificationError)
				Expect(err).To(HaveOccurred())
				Expect(isInvalidNotificationError).To(BeTrue())
			})
		})
	})

	Context("Getting notifications", func() {
		When("getting an existing notification", func() {
			JustBeforeEach(func() {
				nn, err = mgr.GetNotification(ref)
			})

			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return a notification", func() {
				Expect(nn.Ref).To(Equal(n1.Ref))
				Expect(nn.TemplateName).To(Equal(n1.TemplateName))
				Expect(nn.Recipients).To(Equal(n1.Recipients))
			})
		})

		When("getting a non-existing notification", func() {
			JustBeforeEach(func() {
				nn, err = mgr.GetNotification("non-existent-ref")
			})

			It("should return a NotFoundError", func() {
				_, isNotFoundError := err.(*notification.NotFoundError)
				Expect(err).To(HaveOccurred())
				Expect(isNotFoundError).To(BeTrue())
			})
		})
	})

	Context("Deleting notifications", func() {
		When("deleting an existing notification", func() {
			JustBeforeEach(func() {
				err = mgr.DeleteNotification(ref)

			})

			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should delete the notification from the database", func() {
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM cbox_notifications WHERE ref = ?", ref).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(BeZero())
			})

			It("should cascade the deletions to notification_recipients table", func() {
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM cbox_notification_recipients WHERE notification_id = ?", 1).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(BeZero())
			})
		})

		When("deleting a non-existing notification", func() {
			JustBeforeEach(func() {
				err = mgr.DeleteNotification("non-existent-ref")

			})

			It("should not change the db and return a NotFoundError error", func() {
				Expect(err).To(HaveOccurred())
				isNotFoundError, _ := err.(*notification.NotFoundError)
				Expect(isNotFoundError).ToNot(BeNil())
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM cbox_notifications").Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(1))
			})
		})

	})

})
