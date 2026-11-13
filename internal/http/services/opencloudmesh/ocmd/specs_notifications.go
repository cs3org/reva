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

import (
	"bytes"
	"encoding/json"
	"errors"
	"unicode/utf8"

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
	NotificationContactRemoved        = "CONTACT_REMOVED"
	NotificationUserRemoved           = "USER_REMOVED"
	NotificationGroupRemoved          = "GROUP_REMOVED"
)

// NotificationClass describes the validation result.
type NotificationClass string

const (
	NotificationClassSpec      NotificationClass = "known"
	NotificationClassUnknown   NotificationClass = "unknown"
	NotificationClassMalformed NotificationClass = "malformed"
	NotificationClassRequest   NotificationClass = "unsupported"
)

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
	UserID       string `json:"userId"`
	TargetUserID string `json:"targetUserId,omitempty"`
}

// NotificationGroup is the group object of recipient notifications.
type NotificationGroup struct {
	GroupID string `json:"groupId"`
}

// Notification holds parsed fields; formatting omits received values.
type Notification struct {
	Type  string
	Class NotificationClass

	SenderDomain string
	ResourceType string
	ShareType    string
	// ProviderID is notification.file.providerId.
	ProviderID string
	Message    string

	File  *NotificationFile
	User  *NotificationUser
	Group *NotificationGroup

	// Payload holds opaque notification members. Do not log it.
	Payload map[string]json.RawMessage `json:"-"`

	// Retain presence so null and empty values are not treated as absent.
	members map[string]json.RawMessage
}

func (n Notification) String() string {
	return "notification{class=" + string(n.Class) + "}"
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

// Restrict error paths to avoid reflecting payload member names.
var notificationFields = map[string]bool{
	"senderDomain": true, "resourceType": true, "shareType": true, "resourceType or shareType": true,
	"providerId": true, "notification": true, "notification.message": true,
	"notification.file": true, "notification.file.providerId": true,
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

// NotificationReason returns safe text without wrapped error details.
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

func decodeNotification(body []byte) (*Notification, error) {
	// encoding/json replaces invalid UTF-8, which would change opaque identities.
	if !utf8.Valid(body) {
		return nil, &NotificationError{Code: NotificationErrMalformed}
	}
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
	if n.Type == "" {
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
	// The deprecated envelope selector cannot replace file.providerId.
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
		default:
			// The member name is chosen by the sender, so it is not reported.
			if !isJSONObject(value) {
				return invalidField("notification")
			}
		}
	}
	return nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func isJSONObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && raw[0] == '{'
}

// A supplied null or empty string is invalid, even for optional members.
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

func (n *Notification) hasMember(key string) bool {
	_, ok := n.members[key]
	return ok
}

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

func payloadString(obj map[string]json.RawMessage, object, key string) (string, error) {
	return memberString(obj, key, "notification."+object+"."+key, true)
}

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

// REQUEST_SHARE uses shareId instead of the provider's grant identifier.
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
	if _, ok := user["targetUserId"]; ok {
		return invalidField("notification.user.targetUserId")
	}
	id, err := payloadAddress(user, "user", "userId")
	if err != nil {
		return err
	}
	n.User = &NotificationUser{UserID: id}
	return nil
}

func validateContactRemoved(n *Notification) error {
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
	target, err := payloadAddress(user, "user", "targetUserId")
	if err != nil {
		return err
	}
	n.User = &NotificationUser{UserID: id, TargetUserID: target}
	return nil
}

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

// ValidateNotificationFunc fills typed fields or returns a NotificationError.
type ValidateNotificationFunc func(n *Notification) error

var builtinContracts = map[string]ValidateNotificationFunc{
	NotificationShareAccepted:         validateShareState,
	NotificationShareDeclined:         validateShareState,
	NotificationShareUnshared:         validateShareState,
	NotificationShareChangePermission: validateShareState,
	NotificationContactRemoved:        validateContactRemoved,
	NotificationUserRemoved:           validateUserRemoved,
	NotificationGroupRemoved:          validateGroupRemoved,
}

var requestContracts = map[string]ValidateNotificationFunc{
	NotificationRequestShare:   validateRequestShare,
	NotificationRequestReshare: validateRequestReshare,
}
