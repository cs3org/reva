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

package notification

import (
	"fmt"

	"github.com/cs3org/reva/pkg/notification/template"
)

// Notification is the representation of a notification.
type Notification struct {
	TemplateName string
	Template     template.Template
	Ref          string
	Recipients   []string
}

// Manager is the interface notification storage managers have to implement.
type Manager interface {
	// UpsertNotification insert or updates a notification.
	UpsertNotification(n Notification) error
	// GetNotification reads a notification.
	GetNotification(ref string) (*Notification, error)
	// DeleteNotification deletes a notifcation.
	DeleteNotification(ref string) error
}

// NotFoundError is the error returned when a notification does not exist.
type NotFoundError struct {
	Ref string
}

// InvalidNotificationError is the error returned when a notification has invalid data.
type InvalidNotificationError struct {
	Ref string
	Msg string
	Err error
}

// Error returns the string error msg for NotFoundError.
func (n NotFoundError) Error() string {
	return fmt.Sprintf("notification %s not found", n.Ref)
}

// Error returns the string error msg for InvalidNotificationError.
func (i InvalidNotificationError) Error() string {
	return i.Msg
}

// Send is the method run when a notification is triggered.
func (n *Notification) Send(sender string, templateData map[string]interface{}) error {
	subject, err := n.Template.RenderSubject(templateData)
	if err != nil {
		return err
	}

	body, err := n.Template.RenderBody(templateData)
	if err != nil {
		return err
	}

	for _, recipient := range n.Recipients {
		err := n.Template.Handler.Send(sender, recipient, subject, body)
		if err != nil {
			return err
		}
	}

	return nil
}

// CheckNotification checks if a notification has correct data.
func (n *Notification) CheckNotification() error {
	if len(n.Ref) == 0 {
		return &InvalidNotificationError{
			Ref: n.Ref,
			Msg: "empty ref",
		}
	}

	if err := template.CheckTemplateName(n.TemplateName); err != nil {
		return &InvalidNotificationError{
			Ref: n.Ref,
			Msg: fmt.Sprintf("invalid template name %s", n.TemplateName),
			Err: err,
		}
	}

	if len(n.Recipients) == 0 {
		return &InvalidNotificationError{
			Ref: n.Ref,
			Msg: "empty recipient list",
		}
	}

	return nil
}
