// Copyright 2018-2026 CERN
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
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
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
	NotificationServerAdded           = "OCM_SERVER_ADDED"
	NotificationServerRemoved         = "OCM_SERVER_REMOVED"
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
	Message      string

	File  *NotificationFile
	User  *NotificationUser
	Group *NotificationGroup

	// Payload holds opaque notification members. Do not log it.
	Payload map[string]json.RawMessage `json:"-"`
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
// validator field paths, never received values or member names.
type NotificationError struct {
	Code  NotificationErrorCode
	field string
}

func (e *NotificationError) Error() string {
	switch e.Code {
	case NotificationErrMalformed:
		return "malformed notification"
	case NotificationErrMissingType:
		return "missing notificationType"
	case NotificationErrMissingField:
		return "missing " + e.field
	case NotificationErrInvalidField:
		return "invalid " + e.field
	case NotificationErrNotImplemented:
		return "notification type not implemented"
	default:
		return "invalid notification"
	}
}

func missingField(field string) error {
	return &NotificationError{Code: NotificationErrMissingField, field: field}
}

func invalidField(field string) error {
	return &NotificationError{Code: NotificationErrInvalidField, field: field}
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
	n := &Notification{}
	rawType, ok := members["notificationType"]
	if !ok || isJSONNull(rawType) {
		return nil, &NotificationError{Code: NotificationErrMissingType}
	}
	if json.Unmarshal(rawType, &n.Type) != nil || hasLoneSurrogate(rawType, n.Type) {
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
	if message, ok := n.Payload["message"]; ok {
		if isJSONNull(message) || json.Unmarshal(message, &n.Message) != nil {
			return invalidField("notification.message")
		}
	}
	for key, value := range n.Payload {
		// The member name is chosen by the sender, so it is not reported.
		if key != "message" && !isJSONObject(value) {
			return invalidField("notification")
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
	if isJSONNull(raw) || json.Unmarshal(raw, &s) != nil || s == "" || hasLoneSurrogate(raw, s) {
		return "", invalidField(field)
	}
	return s, nil
}

// The schema does not restrict these identifiers, so an empty string is valid.
func memberText(obj map[string]json.RawMessage, key, field string, required bool) error {
	raw, ok := obj[key]
	if !ok {
		if required {
			return missingField(field)
		}
		return nil
	}
	var s string
	if isJSONNull(raw) || json.Unmarshal(raw, &s) != nil {
		return invalidField(field)
	}
	return nil
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

// encoding/json decodes a lone surrogate escape such as \ud800 to U+FFFD, so
// two different identifiers could become one string. raw must be the valid
// JSON string that decoded came from.
func hasLoneSurrogate(raw []byte, decoded string) bool {
	if !strings.ContainsRune(decoded, utf8.RuneError) {
		return false
	}
	// The decoder already checked the escape syntax and the hex digits.
	unit := func(i int) rune {
		v, _ := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		return rune(v)
	}
	for i := 0; i+6 <= len(raw); i++ {
		switch {
		case raw[i] != '\\':
		case raw[i+1] != 'u':
			i++ // skip the escaped byte, which can be a backslash
		case !utf16.IsSurrogate(unit(i)):
			i += 5
		case i+12 <= len(raw) && raw[i+6] == '\\' && raw[i+7] == 'u' &&
			utf16.DecodeRune(unit(i), unit(i+6)) != unicode.ReplacementChar:
			i += 11
		default:
			return true
		}
	}
	return false
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

// An absent list returns nil. A present empty list returns a non-nil empty slice.
func filePermissions(file map[string]json.RawMessage) ([]string, error) {
	const field = "notification.file.permissions"
	raw, ok := file["permissions"]
	if !ok {
		return nil, nil
	}
	var elems []json.RawMessage
	if isJSONNull(raw) || json.Unmarshal(raw, &elems) != nil {
		return nil, invalidField(field)
	}
	perms := make([]string, 0, len(elems))
	for _, e := range elems {
		var p string
		if isJSONNull(e) || json.Unmarshal(e, &p) != nil || !isNotificationPermission(p) {
			return nil, invalidField(field)
		}
		perms = append(perms, p)
	}
	return perms, nil
}

func isNotificationPermission(permission string) bool {
	switch permission {
	case "read", "create", "update", "delete", "write", "share", "view":
		return true
	}
	return false
}

// Share grants are never collections or federation memberships.
func isReservedGrantResourceType(resourceType string) bool {
	return resourceType == "collection" || resourceType == "federation"
}

func (n *Notification) requireGrantResourceType() error {
	if n.ResourceType == "" {
		return missingField("resourceType")
	}
	if isReservedGrantResourceType(n.ResourceType) {
		return invalidField("resourceType")
	}
	return nil
}

func (n *Notification) requireResourceType(want string) error {
	switch n.ResourceType {
	case want:
		return nil
	case "":
		return missingField("resourceType")
	}
	return invalidField("resourceType")
}

func validateShareState(n *Notification) error {
	if err := n.requireGrantResourceType(); err != nil {
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
	if f.Permissions, err = filePermissions(file); err != nil {
		return err
	}
	n.File = f
	return nil
}

func validateRequestReshare(n *Notification) error {
	if err := n.requireGrantResourceType(); err != nil {
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
	if f.Permissions, err = filePermissions(file); err != nil {
		return err
	}
	n.File = f
	return nil
}

func validateServerMembership(n *Notification) error {
	if err := n.requireResourceType("federation"); err != nil {
		return err
	}
	if n.ShareType != "" {
		return invalidField("shareType")
	}
	federation, err := n.payloadObject("federation")
	if err != nil {
		return err
	}
	if err := memberText(federation, "federationId", "notification.federation.federationId", true); err != nil {
		return err
	}
	rawURL, err := payloadString(federation, "federation", "url")
	if err != nil {
		return err
	}
	parsed, err := url.Parse(rawURL)
	const urlField = "notification.federation.url"
	if err != nil || requireAbsoluteHTTPURL(parsed) != nil {
		return invalidField(urlField)
	}
	if _, err := providerdomain.CanonicalOCMHost(parsed.Host); err != nil {
		return invalidField(urlField)
	}
	if path := parsed.EscapedPath(); path != "" && path != "/" {
		return invalidField(urlField)
	}
	if strings.ContainsAny(rawURL, "?#") {
		return invalidField(urlField)
	}
	for _, key := range []string{"name", "mlsGroupId"} {
		if err := memberText(federation, key, "notification.federation."+key, false); err != nil {
			return err
		}
	}
	return nil
}

func validateUserRemoved(n *Notification) error {
	if n.ShareType != "user" {
		return invalidField("shareType")
	}
	if n.ResourceType != "" {
		return invalidField("resourceType")
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
	if n.ResourceType != "" {
		return invalidField("resourceType")
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
	if n.ResourceType != "" {
		return invalidField("resourceType")
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

// Valid events of these types are recognized but have no handler.
var requestContracts = map[string]ValidateNotificationFunc{
	NotificationRequestShare:   nil,
	NotificationRequestReshare: validateRequestReshare,
	NotificationServerAdded:    validateServerMembership,
	NotificationServerRemoved:  validateServerMembership,
}
