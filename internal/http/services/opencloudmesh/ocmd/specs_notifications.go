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

	"github.com/go-playground/validator/v10"
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

// NotificationReshareUndo is a legacy Nextcloud type validated like
// SHARE_UNSHARED. It is about an outgoing share, so it never withdraws a received one.
const NotificationReshareUndo = "RESHARE_UNDO"

// Notification types defined by the OCM-MLS draft. They are validated
// strictly by the MLS ingress, never by the legacy normalizer.
const (
	NotificationMLSWelcome     = "MLS_WELCOME"
	NotificationMLSProposal    = "MLS_PROPOSAL"
	NotificationMLSCommit      = "MLS_COMMIT"
	NotificationMLSApplication = "MLS_APPLICATION"
	NotificationMLSRejoin      = "MLS_REJOIN"
)

// NotificationClass is the triage class of a received notification.
type NotificationClass string

const (
	// NotificationClassSpec is a known type in the specified shape.
	NotificationClassSpec NotificationClass = "A"
	// NotificationClassLegacy is a known type normalized from a legacy shape.
	NotificationClassLegacy NotificationClass = "B"
	// NotificationClassUnknown is a type without a registered contract.
	NotificationClassUnknown NotificationClass = "C"
	// NotificationClassMalformed is a body that cannot be decoded.
	NotificationClassMalformed NotificationClass = "D"
	// NotificationClassMLS is one of the MLS types, reserved for strict validation.
	NotificationClassMLS NotificationClass = "M"
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
	ProviderID  string   `json:"providerId"`
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

// NotificationShape records how a received payload was mapped to the
// canonical form. It never contains payload values.
type NotificationShape struct {
	MissingSenderDomain bool `json:"missingSenderDomain,omitempty"`
	EnvelopeProviderID  bool `json:"envelopeProviderId,omitempty"`
	ProviderIDConflict  bool `json:"providerIdConflict,omitempty"`
	Alias               bool `json:"alias,omitempty"`
}

// Legacy reports whether any legacy mapping was applied.
func (s NotificationShape) Legacy() bool {
	return s.MissingSenderDomain || s.EnvelopeProviderID || s.Alias
}

// Notification is the canonical form of a received notification that every
// TypeHandler consumes. Formatting it prints fixed metadata only.
type Notification struct {
	// Type is the notificationType as received. Aliases keep their identity.
	Type string
	// ContractType is the type whose contract validated the payload, e.g.
	// SHARE_UNSHARED for RESHARE_UNDO. Empty for unknown types.
	ContractType string
	Class        NotificationClass
	Shape        NotificationShape

	SenderDomain string
	ResourceType string
	ShareType    string
	// ProviderID is notification.file.providerId, or the deprecated envelope
	// providerId when the nested one is absent.
	ProviderID   string
	Message      string
	SharedSecret NotificationSecret

	File  *NotificationFile
	User  *NotificationUser
	Group *NotificationGroup

	// Payload holds the raw members of the notification object, for
	// extension and MLS contracts, without the sharedSecret. It is never logged.
	Payload map[string]json.RawMessage `json:"-"`

	rawSecret json.RawMessage
	// members are the raw envelope members, to tell an absent field from a null or empty one.
	members map[string]json.RawMessage
}

func (n Notification) String() string {
	return fmt.Sprintf("notification{type=%q contract=%q class=%s legacy=%t}",
		n.Type, n.ContractType, n.Class, n.Shape.Legacy())
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

func (e *NotificationError) Error() string {
	switch e.Code {
	case NotificationErrMalformed:
		return "malformed notification"
	case NotificationErrMissingType:
		return "missing notificationType"
	case NotificationErrMissingField:
		return "missing " + e.Field
	case NotificationErrInvalidField:
		return "invalid " + e.Field
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

// NotificationErrorCodeOf returns the code of a contract error, if any.
func NotificationErrorCodeOf(err error) (NotificationErrorCode, bool) {
	var ne *NotificationError
	if errors.As(err, &ne) {
		return ne.Code, true
	}
	return "", false
}

// notificationEnvelope captures the spec layout and the legacy flat fields.
type notificationEnvelope struct {
	NotificationType string          `json:"notificationType"`
	SenderDomain     string          `json:"senderDomain"`
	ResourceType     string          `json:"resourceType"`
	ShareType        string          `json:"shareType"`
	ProviderID       string          `json:"providerId"`
	Notification     json.RawMessage `json:"notification"`
}

// decodeNotification decodes the envelope only. Any failure here is class D.
func decodeNotification(body []byte) (*Notification, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil || members == nil {
		return nil, &NotificationError{Code: NotificationErrMalformed}
	}
	rawType, ok := members["notificationType"]
	if !ok || isJSONNull(rawType) {
		return nil, &NotificationError{Code: NotificationErrMissingType}
	}
	var env notificationEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, &NotificationError{Code: NotificationErrMalformed}
	}
	if strings.TrimSpace(env.NotificationType) == "" {
		return nil, &NotificationError{Code: NotificationErrMissingType}
	}

	n := &Notification{
		Type:         env.NotificationType,
		SenderDomain: env.SenderDomain,
		ResourceType: env.ResourceType,
		ShareType:    env.ShareType,
		ProviderID:   env.ProviderID,
		members:      members,
	}
	if len(env.Notification) > 0 && !isJSONNull(env.Notification) {
		if err := json.Unmarshal(env.Notification, &n.Payload); err != nil || n.Payload == nil {
			return nil, &NotificationError{Code: NotificationErrMalformed}
		}
		// Keep the secret out of the raw payload so that no raw view leaks it.
		if raw, ok := n.Payload["sharedSecret"]; ok {
			n.rawSecret = raw
			delete(n.Payload, "sharedSecret")
		}
	}
	return n, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// hasMember reports whether the envelope has the member, even if null or empty.
func (n *Notification) hasMember(key string) bool {
	_, ok := n.members[key]
	return ok
}

// decodeMember decodes a present member into out. A null value is invalid,
// because no known member is nullable.
func decodeMember(raw json.RawMessage, field string, out any) error {
	if isJSONNull(raw) || json.Unmarshal(raw, out) != nil {
		return invalidField(field)
	}
	return nil
}

// payloadObject decodes a member of the notification object into out.
func (n *Notification) payloadObject(key string, out any) (bool, error) {
	raw, ok := n.Payload[key]
	if !ok {
		return false, nil
	}
	return true, decodeMember(raw, "notification."+key, out)
}

// normalizeCommon maps the fields shared by the processable known types. It
// never applies to MLS types, whose bytes feed cryptographic processing.
func (n *Notification) normalizeCommon() error {
	for _, key := range []string{"senderDomain", "resourceType", "shareType", "providerId"} {
		if raw, ok := n.members[key]; ok && isJSONNull(raw) {
			return invalidField(key)
		}
	}
	if n.SenderDomain == "" {
		n.Shape.MissingSenderDomain = true
	}
	if n.rawSecret != nil {
		var secret string
		if err := decodeMember(n.rawSecret, "notification.sharedSecret", &secret); err != nil {
			return err
		}
		n.SharedSecret = NotificationSecret(secret)
	}
	if raw, ok := n.Payload["message"]; ok {
		if err := decodeMember(raw, "notification.message", &n.Message); err != nil {
			return err
		}
	}
	return nil
}

// normalizeFile resolves file.providerId before the deprecated envelope one.
func (n *Notification) normalizeFile() error {
	var f NotificationFile
	present, err := n.payloadObject("file", &f)
	if err != nil {
		return err
	}
	envelope := n.ProviderID
	switch {
	case present && f.ProviderID != "":
		if envelope != "" && envelope != f.ProviderID {
			n.Shape.ProviderIDConflict = true
		}
		n.ProviderID = f.ProviderID
	case envelope != "":
		n.Shape.EnvelopeProviderID = true
		f.ProviderID = envelope
		present = true
	}
	if present {
		n.File = &f
	}
	return nil
}

var notificationValidate = newNotificationValidator()

func newNotificationValidator() *validator.Validate {
	v := validator.New()
	_ = v.RegisterValidation("ocmaddress", func(fl validator.FieldLevel) bool {
		_, _, ok := splitOCMAddress(fl.Field().String())
		return ok
	})
	_ = v.RegisterValidation("ocmpermission", func(fl validator.FieldLevel) bool {
		switch fl.Field().String() {
		case "read", "write", "share":
			return true
		}
		return false
	})
	return v
}

// splitOCMAddress splits identifier "@" host [":" port] at the last "@".
// The identifier is opaque and kept as is; the host follows RFC 3986.
func splitOCMAddress(addr string) (id, host string, ok bool) {
	i := strings.LastIndex(addr, "@")
	if i <= 0 {
		return "", "", false
	}
	id, host = addr[:i], addr[i+1:]
	return id, host, validOCMHost(host)
}

// validOCMHost accepts an IP literal, an IPv4 address or a registered name,
// with an optional port. Paths, queries, userinfo and spaces are rejected.
func validOCMHost(h string) bool {
	name, port := h, ""
	if strings.HasPrefix(h, "[") {
		end := strings.IndexByte(h, ']')
		if end < 0 || !validIPLiteral(h[1:end]) {
			return false
		}
		name, port = "", h[end+1:]
		if port != "" && port[0] != ':' {
			return false
		}
	} else if i := strings.IndexByte(h, ':'); i >= 0 {
		name, port = h[:i], h[i:]
	}
	if port != "" && (len(port) == 1 || strings.Trim(port[1:], "0123456789") != "") {
		return false
	}
	if strings.HasPrefix(h, "[") {
		return true
	}
	return name != "" && validRegName(name)
}

func validIPLiteral(s string) bool {
	if strings.HasPrefix(s, "v") || strings.HasPrefix(s, "V") {
		dot := strings.IndexByte(s, '.')
		return dot > 1 && strings.Trim(s[1:dot], hexDigits) == "" && dot < len(s)-1 &&
			strings.Trim(s[dot+1:], unreservedOrSubDelims+":") == ""
	}
	return s != "" && strings.Trim(s, hexDigits+":.") == ""
}

const (
	hexDigits             = "0123456789abcdefABCDEF"
	unreservedOrSubDelims = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~!$&'()*+,;="
)

// validRegName accepts unreserved, sub-delims and percent-encoded octets.
func validRegName(s string) bool {
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '%':
			if i+2 >= len(s) || !strings.ContainsRune(hexDigits, rune(s[i+1])) || !strings.ContainsRune(hexDigits, rune(s[i+2])) {
				return false
			}
			i += 2
		case !strings.ContainsRune(unreservedOrSubDelims, rune(s[i])):
			return false
		}
	}
	return true
}

// Per-type schemas, checked with the same validator tags as the other ocmd specs.
type shareStateSchema struct {
	ResourceType string   `validate:"required"`
	ProviderID   string   `validate:"required"`
	Permissions  []string `validate:"dive,ocmpermission"`
}

type userRemovedSchema struct {
	ShareType string `validate:"required"`
	UserID    string `validate:"required,ocmaddress"`
}

type groupRemovedSchema struct {
	ShareType string `validate:"required"`
	GroupID   string `validate:"required"`
}

// validateSchema maps validator failures to fixed contract errors.
func validateSchema(schema any) error {
	err := notificationValidate.Struct(schema)
	if err == nil {
		return nil
	}
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) || len(verrs) == 0 {
		return &NotificationError{Code: NotificationErrInvalidField, Field: "notification"}
	}
	fe := verrs[0]
	field := schemaFieldName(fe.StructField())
	if fe.Tag() == "required" {
		return missingField(field)
	}
	return invalidField(field)
}

func schemaFieldName(structField string) string {
	structField, _, _ = strings.Cut(structField, "[")
	switch structField {
	case "ResourceType":
		return "resourceType"
	case "ShareType":
		return "shareType"
	case "ProviderID":
		return "notification.file.providerId"
	case "Permissions":
		return "notification.file.permissions"
	case "UserID":
		return "notification.user.userId"
	case "GroupID":
		return "notification.group.groupId"
	default:
		return "notification"
	}
}

// requireResourceOrShareType applies the either-or rule; both are permitted.
func (n *Notification) requireResourceOrShareType() error {
	if n.ResourceType == "" && n.ShareType == "" {
		return missingField("resourceType or shareType")
	}
	return nil
}

// validateShareState is the contract of the share acceptance and updating
// notifications, shared by the RESHARE_UNDO alias.
func validateShareState(n *Notification) error {
	if err := n.normalizeCommon(); err != nil {
		return err
	}
	if err := n.normalizeFile(); err != nil {
		return err
	}
	schema := shareStateSchema{ResourceType: n.ResourceType, ProviderID: n.ProviderID}
	if n.File != nil {
		schema.Permissions = n.File.Permissions
	}
	return validateSchema(schema)
}

// validateUserRemoved is the contract of USER_REMOVED. The userId belongs to
// the sending server and names no counterpart, so this type stays log-only.
func validateUserRemoved(n *Notification) error {
	if err := n.normalizeCommon(); err != nil {
		return err
	}
	if n.hasMember("resourceType") {
		return invalidField("resourceType")
	}
	var u NotificationUser
	if _, err := n.payloadObject("user", &u); err != nil {
		return err
	}
	if err := validateSchema(userRemovedSchema{ShareType: n.ShareType, UserID: u.UserID}); err != nil {
		return err
	}
	n.User = &u
	return nil
}

// validateGroupRemoved is the contract of GROUP_REMOVED.
func validateGroupRemoved(n *Notification) error {
	if err := n.normalizeCommon(); err != nil {
		return err
	}
	if n.hasMember("resourceType") {
		return invalidField("resourceType")
	}
	var g NotificationGroup
	if _, err := n.payloadObject("group", &g); err != nil {
		return err
	}
	if err := validateSchema(groupRemovedSchema{ShareType: n.ShareType, GroupID: g.GroupID}); err != nil {
		return err
	}
	n.Group = &g
	return nil
}

// ValidateNotificationFunc validates and normalizes a known notification in
// place. It returns a *NotificationError for schema failures.
type ValidateNotificationFunc func(n *Notification) error

// builtinContracts maps the processable known types to their contract.
// The alias entry validates RESHARE_UNDO against the SHARE_UNSHARED contract.
var builtinContracts = map[string]struct {
	contractType string
	validate     ValidateNotificationFunc
}{
	NotificationShareAccepted:         {NotificationShareAccepted, validateShareState},
	NotificationShareDeclined:         {NotificationShareDeclined, validateShareState},
	NotificationShareUnshared:         {NotificationShareUnshared, validateShareState},
	NotificationShareChangePermission: {NotificationShareChangePermission, validateShareState},
	NotificationReshareUndo:           {NotificationShareUnshared, validateShareState},
	NotificationUserRemoved:           {NotificationUserRemoved, validateUserRemoved},
	NotificationGroupRemoved:          {NotificationGroupRemoved, validateGroupRemoved},
}

// IsRequestNotification reports the request types that are never fulfilled.
func IsRequestNotification(notificationType string) bool {
	return notificationType == NotificationRequestShare || notificationType == NotificationRequestReshare
}

// IsMLSNotification reports the five OCM-MLS notification types.
func IsMLSNotification(notificationType string) bool {
	switch notificationType {
	case NotificationMLSWelcome, NotificationMLSProposal, NotificationMLSCommit,
		NotificationMLSApplication, NotificationMLSRejoin:
		return true
	}
	return false
}

// finishKnown validates a known type and assigns its A/B class.
func finishKnown(n *Notification, contractType string, validate ValidateNotificationFunc) error {
	n.ContractType = contractType
	n.Shape.Alias = contractType != n.Type
	err := validate(n)
	if err == nil {
		err = n.requireResourceOrShareType()
	}
	// The class is set on failure too, so that diagnostics keep the shape.
	if n.Shape.Legacy() {
		n.Class = NotificationClassLegacy
	} else {
		n.Class = NotificationClassSpec
	}
	return err
}
