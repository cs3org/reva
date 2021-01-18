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

package oidcprovider

// StandardClaims are the standard claims defined in OIDC.
// Section 5.3.2, or in the ID Token, per Section 2.
// see https://openid.net/specs/openid-connect-core-1_0.html#StandardClaims
// TODO(labkode): create PR for the core-os/oidc with StandardClaims public struct.
// TODO(labkode): we need to allow adding custom claims and define the mappings to the user struct.
type StandardClaims struct {
	// Time the End-User's information was last updated. Its value is a
	// JSON number representing the number of seconds from 1970-01-01T0:0:0Z
	// as measured in UTC until the date/time.
	UpdatedAt int64 `json:"updated_at,omitempty"`

	// True if the End-User's e-mail address has been verified; otherwise false.
	// When this Claim Value is true, this means that the OP took affirmative
	// steps to ensure that this e-mail address was controlled by the End-User
	// at the time the verification was performed. The means by which an e-mail
	// address is verified is context-specific, and dependent upon the trust
	// framework or contractual agreements within which the parties are operating.
	EmailVerified bool `json:"email_verified,omitempty"`

	// True if the End-User's phone number has been verified; otherwise false.
	// When this Claim Value is true, this means that the OP took affirmative
	// steps to ensure that this phone number was controlled by the End-User
	// at the time the verification was performed. The means by which a phone
	// number is verified is context-specific, and dependent upon the trust
	// framework or contractual agreements within which the parties are
	// operating. When true, the phone_number Claim MUST be in E.164 format
	// and any extensions MUST be represented in RFC 3966 format.
	PhoneNumberVerified bool `json:"phone_number_verified,omitempty"`

	Iss string `json:"iss"`

	// Subject - Identifier for the End-User at the Issuer.
	Sub string `json:"sub,omitempty"`

	// End-User's full name in displayable form including all name parts, possibly
	// including titles and suffixes, ordered according to the End-User's locale
	// and preferences.
	Name string `json:"name,omitempty"`

	// Given name(s) or first name(s) of the End-User. Note that in some cultures,
	// people can have multiple given names; all can be present, with the names
	// being separated by space characters.
	GivenName string `json:"given_name,omitempty"`

	// Surname(s) or last name(s) of the End-User. Note that in some cultures,
	// people can have multiple family names or no family name; all can be present,
	// with the names being separated by space characters.
	FamilyName string `json:"family_name,omitempty"`

	// Middle name(s) of the End-User. Note that in some cultures, people can have
	// multiple middle names; all can be present, with the names being separated by
	// space characters. Also note that in some cultures, middle names are not used.
	MiddleName string `json:"middle_name,omitempty"`

	// Casual name of the End-User that may or may not be the same as the given_name.
	// For instance, a nickname value of Mike might be returned alongside a given_name
	// value of Michael.
	Nickname string `json:"nickname,omitempty"`

	// Shorthand name by which the End-User wishes to be referred to at the RP, such
	// as janedoe or j.doe. This value MAY be any valid JSON string including special
	// characters such as @, /, or whitespace. The RP MUST NOT rely upon this value
	// being unique, as discussed in Section 5.7.
	PreferredUsername string `json:"preferred_username,omitempty"`

	// URL of the End-User's profile page. The contents of this Web page SHOULD be
	// about the End-User.
	Profile string `json:"profile,omitempty"`

	// URL of the End-User's profile picture. This URL MUST refer to an image file
	// (for example, a PNG, JPEG, or GIF image file), rather than to a Web page
	// containing an image. Note that this URL SHOULD specifically reference a
	// profile photo of the End-User suitable for displaying when describing the
	// End-User, rather than an arbitrary photo taken by the End-User.
	Picture string `json:"picture,omitempty"`

	// URL of the End-User's Web page or blog. This Web page SHOULD contain
	// information published by the End-User or an organization that the End-User
	// is affiliated with.
	Website string `json:"website,omitempty"`

	// End-User's preferred e-mail address. Its value MUST conform to the RFC 5322
	// addr-spec syntax. The RP MUST NOT rely upon this value being unique, as
	// discussed in Section 5.7.
	Email string `json:"email,omitempty"`

	// End-User's gender. Values defined by this specification are female and male.
	// Other values MAY be used when neither of the defined values are applicable.
	Gender string `json:"gender,omitempty"`

	// End-User's birthday, represented as an ISO 8601:2004 YYYY-MM-DD format.
	// The year MAY be 0000, indicating that it is omitted. To represent only the
	// year, YYYY format is allowed. Note that depending on the underlying
	// platform's date related function, providing just year can result in
	// varying month and day, so the implementers need to take this factor into
	// account to correctly process the dates.
	Birthdate string `json:"birthdate,omitempty"`

	// String from zoneinfo time zone database representing the End-User's time
	// zone. For example, Europe/Paris or America/Los_Angeles.
	Zoneinfo string `json:"zoneinfo,omitempty"`

	// End-User's locale, represented as a BCP47 [RFC5646] language tag.
	// This is typically an ISO 639-1 Alpha-2 [ISO639‑1] language code in
	// lowercase and an ISO 3166-1 Alpha-2 [ISO3166‑1] country code in
	// uppercase, separated by a dash. For example, en-US or fr-CA. As a
	// compatibility note, some implementations have used an underscore as
	// the separator rather than a dash, for example, en_US; Relying Parties
	// MAY choose to accept this locale syntax as well.
	Locale string `json:"locale,omitempty"`

	// End-User's preferred telephone number. E.164 [E.164] is RECOMMENDED
	// as the format of this Claim, for example, +1 (425) 555-1212 or
	// +56 (2) 687 2400. If the phone number contains an extension, it is
	// RECOMMENDED that the extension be represented using the RFC 3966
	// extension syntax, for example, +1 (604) 555-1234;ext=5678.
	PhoneNumber string `json:"phone_number,omitempty"`

	// TODO Name is the correct one, does kopano use display name? -> double check and report bug
	DisplayName string `json:"display_name,omitempty"`

	Groups []string `json:"groups,omitempty"`

	// End-User's preferred postal address. The value of the address member
	// is a JSON [RFC4627] structure containing some or all of the members
	// defined in Section 5.1.1.
	// TODO add address claim https://openid.net/specs/openid-connect-core-1_0.html#AddressClaim
	Address map[string]interface{} `json:"address,omitempty"`
}

/*
// The IntrospectionResponse is a JSON object [RFC7159] in
// "application/json" format with the following top-level members.
// see https://tools.ietf.org/html/rfc7662#section-2.2
type IntrospectionResponse struct {
	// REQUIRED.  Boolean indicator of whether or not the presented token
	// is currently active.  The specifics of a token's "active" state
	// will vary depending on the implementation of the authorization
	// server and the information it keeps about its tokens, but a "true"
	// value return for the "active" property will generally indicate
	// that a given token has been issued by this authorization server,
	// has not been revoked by the resource owner, and is within its
	// given time window of validity (e.g., after its issuance time and
	// before its expiration time).  See Section 4 for information on
	// implementation of such checks.
	Active bool `json:"active"`
	// OPTIONAL.  A JSON string containing a space-separated list of
	// scopes associated with this token, in the format described in
	// Section 3.3 of OAuth 2.0 [RFC6749].
	Scope string `json:"scope,omitempty"`
	// OPTIONAL.  Client identifier for the OAuth 2.0 client that
	// requested this token.
	ClientID string `json:"client_id,omitempty"`
	// OPTIONAL.  Human-readable identifier for the resource owner who
	// authorized this token.
	Username string `json:"username,omitempty"`
	// OPTIONAL.  Type of the token as defined in Section 5.1 of OAuth
	// 2.0 [RFC6749].
	TokenType string `json:"token_type,omitempty"`
	// OPTIONAL.  Integer timestamp, measured in the number of seconds
	// since January 1 1970 UTC, indicating when this token will expire,
	// as defined in JWT [RFC7519].
	Exp int64 `json:"exp,omitempty"`
	// OPTIONAL.  Integer timestamp, measured in the number of seconds
	// since January 1 1970 UTC, indicating when this token was
	// originally issued, as defined in JWT [RFC7519].
	Iat int64 `json:"iat,omitempty"`
	// OPTIONAL.  Integer timestamp, measured in the number of seconds
	// since January 1 1970 UTC, indicating when this token is not to be
	// used before, as defined in JWT [RFC7519].
	Nbf int64 `json:"nbf,omitempty"`
	// OPTIONAL.  Subject of the token, as defined in JWT [RFC7519].
	// Usually a machine-readable identifier of the resource owner who
	// authorized this token.
	Sub string `json:"sub,omitempty"`
	// OPTIONAL.  Service-specific string identifier or list of string
	// identifiers representing the intended audience for this token, as
	// defined in JWT [RFC7519].
	Aud string `json:"aud,omitempty"`
	// OPTIONAL.  String representing the issuer of this token, as
	// defined in JWT [RFC7519].
	Iss string `json:"iss,omitempty"`
	// OPTIONAL.  String identifier for the token, as defined in JWT [RFC7519].
	Jti string `json:"jti,omitempty"`
}
*/
