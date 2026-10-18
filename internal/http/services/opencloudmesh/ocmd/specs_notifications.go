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

// This file holds the portable OCM notification contract: the canonical
// notification, legacy normalization and per-type validation. It must not
// import reva adapters, gateway clients or HTTP wiring, so that other
// implementations can reuse it as is.

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

// NotificationReshareUndo is a legacy Nextcloud type without a registered
// successor. It is validated like SHARE_UNSHARED but keeps its own identity,
// because it is sent to the owner about an outgoing share and must never be
// handled as the withdrawal of a received share.
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

// Reveal returns the secret itself, for comparison only.
func (s NotificationSecret) Reveal() string { return string(s) }

// String implements fmt.Stringer.
func (s NotificationSecret) String() string {
	if s == "" {
		return ""
	}
	return redactedSecret
}

// GoString implements fmt.GoStringer.
func (s NotificationSecret) GoString() string { return s.String() }

// MarshalText implements encoding.TextMarshaler.
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
// TypeHandler consumes.
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
}

// String implements fmt.Stringer with fixed metadata only, so formatting a
// notification never prints payload values or the secret.
func (n Notification) String() string {
	return fmt.Sprintf("notification{type=%q contract=%q class=%s legacy=%t}",
		n.Type, n.ContractType, n.Class, n.Shape.Legacy())
}

// GoString implements fmt.GoStringer.
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

// payloadString reads an optional string member of the notification object.
func (n *Notification) payloadString(key string) (string, bool, error) {
	raw, ok := n.Payload[key]
	if !ok || isJSONNull(raw) {
		return "", false, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", true, invalidField("notification." + key)
	}
	return s, true, nil
}

// payloadObject decodes a member of the notification object into out.
func (n *Notification) payloadObject(key string, out any) (bool, error) {
	raw, ok := n.Payload[key]
	if !ok || isJSONNull(raw) {
		return false, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return true, invalidField("notification." + key)
	}
	return true, nil
}

// normalizeCommon maps the fields shared by the processable known types. It
// never applies to MLS types, whose bytes feed cryptographic processing.
func (n *Notification) normalizeCommon() error {
	if n.SenderDomain == "" {
		n.Shape.MissingSenderDomain = true
	}
	if len(n.rawSecret) > 0 && !isJSONNull(n.rawSecret) {
		var secret string
		if err := json.Unmarshal(n.rawSecret, &secret); err != nil {
			return invalidField("notification.sharedSecret")
		}
		n.SharedSecret = NotificationSecret(secret)
	}
	// The message is optional human text; a non-string value is dropped.
	if msg, _, err := n.payloadString("message"); err == nil {
		n.Message = msg
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
		return validOCMAddress(fl.Field().String())
	})
	return v
}

// validOCMAddress checks the <id>@<provider> form with both parts non-empty.
func validOCMAddress(addr string) bool {
	last := strings.LastIndex(addr, "@")
	return last > 0 && last < len(addr)-1 && strings.TrimSpace(addr) == addr
}

// Per-type schemas, checked with the same validator tags as the other ocmd specs.
type shareStateSchema struct {
	ResourceType string `validate:"required"`
	ProviderID   string `validate:"required"`
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
	switch structField {
	case "ResourceType":
		return "resourceType"
	case "ShareType":
		return "shareType"
	case "ProviderID":
		return "notification.file.providerId"
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
	return validateSchema(shareStateSchema{ResourceType: n.ResourceType, ProviderID: n.ProviderID})
}

// validateUserRemoved is the contract of USER_REMOVED. The userId belongs to
// the sending server and names no counterpart, so this type stays log-only.
func validateUserRemoved(n *Notification) error {
	if err := n.normalizeCommon(); err != nil {
		return err
	}
	if n.ResourceType != "" {
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
	if n.ResourceType != "" {
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
