// Copyright 2018-2024 CERN
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

package ocmd

// The notification contract below imports no reva adapter, gateway or HTTP
// wiring, so that other implementations can reuse it.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
)

// Notification types registered in the "OCM Notification Types" IANA registry.
const (
	NotificationShareAccepted         = "SHARE_ACCEPTED"
	NotificationShareDeclined         = "SHARE_DECLINED"
	NotificationShareUnshared         = "SHARE_UNSHARED"
	NotificationShareChangePermission = "SHARE_CHANGE_PERMISSION"
	NotificationRequestReshare        = "REQUEST_RESHARE"
	NotificationRequestShare          = "REQUEST_SHARE"
	NotificationUserRemoved           = "USER_REMOVED"
	NotificationGroupRemoved          = "GROUP_REMOVED"
)

// NotificationClass is the triage class of a received notification.
type NotificationClass string

const (
	// NotificationClassSpec is a known type with a valid payload.
	NotificationClassSpec NotificationClass = "A"
	// NotificationClassUnknown is a type without a registered contract.
	NotificationClassUnknown NotificationClass = "C"
	// NotificationClassMalformed is an invalid envelope or the invalid payload
	// of a registered type.
	NotificationClassMalformed NotificationClass = "D"
	// NotificationClassRequest is a request type that is never fulfilled.
	NotificationClassRequest NotificationClass = "R"
)

// NotificationSecret holds notification.sharedSecret. Its string, Go-syntax,
// JSON and text forms are redacted so that it never reaches a log by accident.
type NotificationSecret string

const redactedSecret = "[redacted]"

// Reveal returns the secret itself. Do not format or log the result.
func (s NotificationSecret) Reveal() string { return string(s) }

func (s NotificationSecret) String() string {
	if s == "" {
		return ""
	}
	return redactedSecret
}

func (s NotificationSecret) GoString() string { return s.String() }

func (s NotificationSecret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// NotificationFile is the file object of resource notifications.
type NotificationFile struct {
	ProviderID  string   `json:"providerId,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	ShareWith   string   `json:"shareWith,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	ShareID     string   `json:"shareId,omitempty"`
}

// NotificationUser is the user object of recipient notifications.
type NotificationUser struct {
	UserID string `json:"userId"`
}

// NotificationGroup is the group object of recipient notifications.
type NotificationGroup struct {
	GroupID string `json:"groupId"`
}

// Notification is the canonical form of a received notification that every
// TypeHandler consumes. Formatting it prints fixed metadata only.
type Notification struct {
	Type  string
	Class NotificationClass

	SenderDomain string
	ResourceType string
	ShareType    string
	// ProviderID is notification.file.providerId.
	ProviderID   string
	Message      string
	SharedSecret NotificationSecret

	File  *NotificationFile
	User  *NotificationUser
	Group *NotificationGroup

	// Payload holds the raw members of the notification object, without the
	// sharedSecret, for registered extensions. It is never logged.
	Payload map[string]json.RawMessage `json:"-"`

	// members are the raw envelope members, to tell an absent field from a null or empty one.
	members map[string]json.RawMessage
}

func (n Notification) String() string {
	return fmt.Sprintf("notification{type=%q class=%s}", n.Type, n.Class)
}

func (n Notification) GoString() string { return n.String() }

// NotificationErrorCode is the closed set of notification contract errors.
type NotificationErrorCode string

const (
	NotificationErrMalformed      NotificationErrorCode = "MALFORMED_NOTIFICATION"
	NotificationErrMissingType    NotificationErrorCode = "MISSING_NOTIFICATION_TYPE"
	NotificationErrMissingField   NotificationErrorCode = "MISSING_FIELD"
	NotificationErrInvalidField   NotificationErrorCode = "INVALID_FIELD"
	NotificationErrNotImplemented NotificationErrorCode = "NOT_IMPLEMENTED"
)

// NotificationError is a contract error. Its text holds fixed tokens and
// field paths only, never received values.
type NotificationError struct {
	Code  NotificationErrorCode
	Field string
}

// notificationFields are the field paths a notification error may name.
// Any other Field, e.g. one built by an extension, renders as notification.
var notificationFields = map[string]bool{
	"senderDomain": true, "resourceType": true, "shareType": true, "resourceType or shareType": true,
	"providerId": true, "notification": true, "notification.message": true,
	"notification.sharedSecret": true, "notification.file": true, "notification.file.providerId": true,
	"notification.file.permissions": true, "notification.file.owner": true, "notification.file.shareWith": true,
	"notification.file.shareId": true, "notification.user": true, "notification.user.userId": true,
	"notification.user.targetUserId": true, "notification.group": true, "notification.group.groupId": true,
}

func (e *NotificationError) field() string {
	if notificationFields[e.Field] {
		return e.Field
	}
	return "notification"
}

func (e *NotificationError) Error() string {
	switch e.Code {
	case NotificationErrMalformed:
		return "malformed notification"
	case NotificationErrMissingType:
		return "missing notificationType"
	case NotificationErrMissingField:
		return "missing " + e.field()
	case NotificationErrInvalidField:
		return "invalid " + e.field()
	case NotificationErrNotImplemented:
		return "notification type not implemented"
	default:
		return "invalid notification"
	}
}

func missingField(field string) error {
	return &NotificationError{Code: NotificationErrMissingField, Field: field}
}

func invalidField(field string) error {
	return &NotificationError{Code: NotificationErrInvalidField, Field: field}
}

// NotificationReason is the fixed text of a parse error for responses and
// logs. It never formats the cause of an error that wraps a contract error.
func NotificationReason(err error) string {
	var ne *NotificationError
	if errors.As(err, &ne) {
		return ne.Error()
	}
	return "invalid notification"
}

// NotificationErrorCodeOf returns the code of a contract error, if any.
func NotificationErrorCodeOf(err error) (NotificationErrorCode, bool) {
	var ne *NotificationError
	if errors.As(err, &ne) {
		return ne.Code, true
	}
	return "", false
}

// decodeNotification decodes and validates the generic envelope, which every
// type must satisfy, including types that are not known.
func decodeNotification(body []byte) (*Notification, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil || members == nil {
		return nil, &NotificationError{Code: NotificationErrMalformed}
	}
	n := &Notification{members: members}
	rawType, ok := members["notificationType"]
	if !ok || isJSONNull(rawType) {
		return nil, &NotificationError{Code: NotificationErrMissingType}
	}
	if json.Unmarshal(rawType, &n.Type) != nil {
		return nil, &NotificationError{Code: NotificationErrMalformed}
	}
	if strings.TrimSpace(n.Type) == "" {
		return nil, &NotificationError{Code: NotificationErrMissingType}
	}
	sender, err := memberString(members, "senderDomain", "senderDomain", true)
	if err != nil {
		return nil, err
	}
	if n.SenderDomain, err = providerdomain.CanonicalOCMHost(sender); err != nil {
		return nil, invalidField("senderDomain")
	}
	if n.ResourceType, err = memberString(members, "resourceType", "resourceType", false); err != nil {
		return nil, err
	}
	if n.ShareType, err = memberString(members, "shareType", "shareType", false); err != nil {
		return nil, err
	}
	if n.ResourceType == "" && n.ShareType == "" {
		return nil, missingField("resourceType or shareType")
	}
	// The deprecated top-level providerId is only type-checked: it never
	// stands in for notification.file.providerId.
	if raw, ok := members["providerId"]; ok {
		var s string
		if isJSONNull(raw) || json.Unmarshal(raw, &s) != nil {
			return nil, invalidField("providerId")
		}
	}
	if raw, ok := members["notification"]; ok {
		if err := n.decodePayload(raw); err != nil {
			return nil, err
		}
	}
	return n, nil
}

// decodePayload checks the generic notification object: message and
// sharedSecret are strings, every other member is an object.
func (n *Notification) decodePayload(raw json.RawMessage) error {
	if isJSONNull(raw) || json.Unmarshal(raw, &n.Payload) != nil {
		return invalidField("notification")
	}
	for key, value := range n.Payload {
		switch key {
		case "message":
			if isJSONNull(value) || json.Unmarshal(value, &n.Message) != nil {
				return invalidField("notification.message")
			}
		case "sharedSecret":
			var secret string
			if isJSONNull(value) || json.Unmarshal(value, &secret) != nil {
				return invalidField("notification.sharedSecret")
			}
			n.SharedSecret = NotificationSecret(secret)
		default:
			// The member name is chosen by the sender, so it is not reported.
			if !isJSONObject(value) {
				return invalidField("notification")
			}
		}
	}
	// Keep the secret out of the raw payload so that no raw view leaks it.
	delete(n.Payload, "sharedSecret")
	return nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func isJSONObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && raw[0] == '{'
}

// memberString reads a string member of obj. A present member must be a
// non-empty string: null and "" are invalid, not absent.
func memberString(obj map[string]json.RawMessage, key, field string, required bool) (string, error) {
	raw, ok := obj[key]
	if !ok {
		if required {
			return "", missingField(field)
		}
		return "", nil
	}
	var s string
	if isJSONNull(raw) || json.Unmarshal(raw, &s) != nil || s == "" {
		return "", invalidField(field)
	}
	return s, nil
}

// hasMember reports whether the envelope has the member, even if null or empty.
func (n *Notification) hasMember(key string) bool {
	_, ok := n.members[key]
	return ok
}

// payloadObject returns the raw members of a required object of the
// notification payload.
func (n *Notification) payloadObject(key string) (map[string]json.RawMessage, error) {
	field := "notification." + key
	raw, ok := n.Payload[key]
	if !ok {
		return nil, missingField(field)
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return nil, invalidField(field)
	}
	return obj, nil
}

// payloadString reads a required non-empty string member of a payload object.
func payloadString(obj map[string]json.RawMessage, object, key string) (string, error) {
	return memberString(obj, key, "notification."+object+"."+key, true)
}

// payloadAddress reads a required OCM address member of a payload object.
func payloadAddress(obj map[string]json.RawMessage, object, key string) (string, error) {
	s, err := payloadString(obj, object, key)
	if err != nil {
		return "", err
	}
	if _, _, err := providerdomain.ParseOCMAddress(s); err != nil {
		return "", invalidField("notification." + object + "." + key)
	}
	return s, nil
}

// payloadPermissions reads file.permissions: an array of read, write and share.
func payloadPermissions(file map[string]json.RawMessage, required bool) ([]string, error) {
	const field = "notification.file.permissions"
	raw, ok := file["permissions"]
	if !ok {
		if required {
			return nil, missingField(field)
		}
		return nil, nil
	}
	var elems []json.RawMessage
	if isJSONNull(raw) || json.Unmarshal(raw, &elems) != nil {
		return nil, invalidField(field)
	}
	perms := make([]string, 0, len(elems))
	for _, e := range elems {
		var p string
		if isJSONNull(e) || json.Unmarshal(e, &p) != nil {
			return nil, invalidField(field)
		}
		switch p {
		case "read", "write", "share":
		default:
			return nil, invalidField(field)
		}
		perms = append(perms, p)
	}
	return perms, nil
}

func (n *Notification) requireResourceType() error {
	if n.ResourceType == "" {
		return missingField("resourceType")
	}
	return nil
}

// forbid rejects a member by presence: null and "" count as present.
func (n *Notification) forbid(key string) error {
	if n.hasMember(key) {
		return invalidField(key)
	}
	return nil
}

// validateShareState is the contract of the share acceptance and updating notifications.
func validateShareState(n *Notification) error {
	if err := n.requireResourceType(); err != nil {
		return err
	}
	file, err := n.payloadObject("file")
	if err != nil {
		return err
	}
	f := &NotificationFile{}
	if f.ProviderID, err = payloadString(file, "file", "providerId"); err != nil {
		return err
	}
	if f.Permissions, err = payloadPermissions(file, false); err != nil {
		return err
	}
	n.File, n.ProviderID = f, f.ProviderID
	return nil
}

// validateRequestReshare is the contract of REQUEST_RESHARE, checked before
// the request is refused as not implemented.
func validateRequestReshare(n *Notification) error {
	if err := n.requireResourceType(); err != nil {
		return err
	}
	file, err := n.payloadObject("file")
	if err != nil {
		return err
	}
	f := &NotificationFile{}
	if f.ProviderID, err = payloadString(file, "file", "providerId"); err != nil {
		return err
	}
	if f.ShareWith, err = payloadAddress(file, "file", "shareWith"); err != nil {
		return err
	}
	if f.Permissions, err = payloadPermissions(file, false); err != nil {
		return err
	}
	n.File, n.ProviderID = f, f.ProviderID
	return nil
}

// validateRequestShare is the contract of REQUEST_SHARE. It names the
// resource by shareId and has no providerId requirement.
func validateRequestShare(n *Notification) error {
	if err := n.requireResourceType(); err != nil {
		return err
	}
	if err := n.forbid("shareType"); err != nil {
		return err
	}
	file, err := n.payloadObject("file")
	if err != nil {
		return err
	}
	f := &NotificationFile{}
	if f.Owner, err = payloadAddress(file, "file", "owner"); err != nil {
		return err
	}
	if f.ShareWith, err = payloadAddress(file, "file", "shareWith"); err != nil {
		return err
	}
	if f.ShareID, err = payloadString(file, "file", "shareId"); err != nil {
		return err
	}
	if f.Permissions, err = payloadPermissions(file, true); err != nil {
		return err
	}
	n.File = f
	return nil
}

// validateUserRemoved is the contract of USER_REMOVED. The userId belongs to
// the sending server and names no counterpart, so this type stays log-only.
func validateUserRemoved(n *Notification) error {
	if n.ShareType != "user" {
		return invalidField("shareType")
	}
	if err := n.forbid("resourceType"); err != nil {
		return err
	}
	user, err := n.payloadObject("user")
	if err != nil {
		return err
	}
	id, err := payloadAddress(user, "user", "userId")
	if err != nil {
		return err
	}
	n.User = &NotificationUser{UserID: id}
	return nil
}

// validateGroupRemoved is the contract of GROUP_REMOVED.
func validateGroupRemoved(n *Notification) error {
	if n.ShareType != "group" {
		return invalidField("shareType")
	}
	if err := n.forbid("resourceType"); err != nil {
		return err
	}
	group, err := n.payloadObject("group")
	if err != nil {
		return err
	}
	id, err := payloadString(group, "group", "groupId")
	if err != nil {
		return err
	}
	n.Group = &NotificationGroup{GroupID: id}
	return nil
}

// ValidateNotificationFunc validates a known notification after its generic
// envelope and fills its typed fields. It returns a *NotificationError.
type ValidateNotificationFunc func(n *Notification) error

// builtinContracts are the known types that can have a handler.
var builtinContracts = map[string]ValidateNotificationFunc{
	NotificationShareAccepted:         validateShareState,
	NotificationShareDeclined:         validateShareState,
	NotificationShareUnshared:         validateShareState,
	NotificationShareChangePermission: validateShareState,
	NotificationUserRemoved:           validateUserRemoved,
	NotificationGroupRemoved:          validateGroupRemoved,
}

// requestContracts are the known request types: valid requests are refused
// as not implemented, malformed ones are rejected.
var requestContracts = map[string]ValidateNotificationFunc{
	NotificationRequestShare:   validateRequestShare,
	NotificationRequestReshare: validateRequestReshare,
}

// IsRequestNotification reports the request types that are never fulfilled.
func IsRequestNotification(notificationType string) bool {
	_, ok := requestContracts[notificationType]
	return ok
}
