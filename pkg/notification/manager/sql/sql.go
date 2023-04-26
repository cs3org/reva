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

package sql

import (
	"database/sql"
	"fmt"

	"github.com/cs3org/reva/pkg/notification"
	"github.com/cs3org/reva/pkg/notification/manager/registry"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("sql", NewMysql)
}

type config struct {
	DBUsername string `mapstructure:"db_username"`
	DBPassword string `mapstructure:"db_password"`
	DBHost     string `mapstructure:"db_host"`
	DBPort     int    `mapstructure:"db_port"`
	DBName     string `mapstructure:"db_name"`
	GatewaySvc string `mapstructure:"gatewaysvc"`
}

type mgr struct {
	driver string
	db     *sql.DB
}

// NewMysql returns an instance of the sql notifications manager.
func NewMysql(m map[string]interface{}) (notification.Manager, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName))
	if err != nil {
		return nil, err
	}

	return New("mysql", db)
}

// New returns a new Notifications driver connecting to the given sql.DB.
func New(driver string, db *sql.DB) (notification.Manager, error) {
	return &mgr{
		driver: driver,
		db:     db,
	}, nil
}

// UpsertNotification creates or updates a notification.
func (m *mgr) UpsertNotification(n notification.Notification) error {
	if err := n.CheckNotification(); err != nil {
		return err
	}

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}

	// Create/update notification
	stmt, err := m.db.Prepare("REPLACE INTO cbox_notifications (ref, template_name) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(n.Ref, n.TemplateName)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	// Create/update recipients for the notification
	notificationID, err := result.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	stmt, err = tx.Prepare("REPLACE INTO cbox_notification_recipients (notification_id, recipient) VALUES (?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, recipient := range n.Recipients {
		_, err := stmt.Exec(notificationID, recipient)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// GetNotification reads a notification.
func (m *mgr) GetNotification(ref string) (*notification.Notification, error) {
	query := `
		SELECT n.id, n.ref, n.template_name, nr.recipient
		FROM cbox_notifications AS n
		JOIN cbox_notification_recipients AS nr ON n.id = nr.notification_id
		WHERE n.ref = ?
	`

	rows, err := m.db.Query(query, ref)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var n notification.Notification
	count := 0
	n.Recipients = make([]string, 0)

	for rows.Next() {
		var id string
		var recipient string
		err := rows.Scan(&id, &n.Ref, &n.TemplateName, &recipient)
		if err != nil {
			return nil, err
		}
		n.Recipients = append(n.Recipients, recipient)
		count++
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, &notification.NotFoundError{
			Ref: n.Ref,
		}
	}

	return &n, nil
}

// DeleteNotification deletes a notification.
func (m *mgr) DeleteNotification(ref string) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}

	// Delete notification
	stmt, err := m.db.Prepare("DELETE FROM cbox_notifications WHERE ref = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(ref)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	if rowsAffected == 0 {
		return &notification.NotFoundError{
			Ref: ref,
		}
	}

	return nil
}
