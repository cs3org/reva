// Copyright 2018-2019 CERN
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

package ocs

import (
	"encoding/xml"
	"net/http"
)

// ocsBool implements the xml/json Marshaler interface. The OCS API inconsistency require us to parse boolean values
// as native booleans for json requests but "truthy" 0/1 values for xml requests.
type ocsBool bool

func (c *ocsBool) MarshalJSON() ([]byte, error) {
	if *c {
		return []byte("true"), nil
	}

	return []byte("false"), nil
}

func (c ocsBool) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if c {
		return e.EncodeElement("1", start)
	}

	return e.EncodeElement("0", start)
}

// CapabilitiesHandler renders the capability endpoint
type CapabilitiesHandler struct {
	c CapabilitiesData
}

func (h *CapabilitiesHandler) init(c *Config) {
	h.c = c.Capabilities

	// capabilities
	if h.c.Capabilities == nil {
		h.c.Capabilities = &Capabilities{}
	}

	// core

	if h.c.Capabilities.Core == nil {
		h.c.Capabilities.Core = &CapabilitiesCore{}
	}
	if h.c.Capabilities.Core.PollInterval == 0 {
		h.c.Capabilities.Core.PollInterval = 60
	}
	if h.c.Capabilities.Core.WebdavRoot == "" {
		h.c.Capabilities.Core.WebdavRoot = "remote.php/webdav"
	}

	if h.c.Capabilities.Core.Status == nil {
		h.c.Capabilities.Core.Status = &Status{}
	}
	// h.c.Capabilities.Core.Status.Installed is boolean
	// h.c.Capabilities.Core.Status.Maintenance is boolean
	// h.c.Capabilities.Core.Status.NeedsDBUpgrade is boolean
	if h.c.Capabilities.Core.Status.Version == "" {
		h.c.Capabilities.Core.Status.Version = "10.0.9.5" // TODO make build determined
	}
	if h.c.Capabilities.Core.Status.VersionString == "" {
		h.c.Capabilities.Core.Status.VersionString = "10.0.9" // TODO make build determined
	}
	if h.c.Capabilities.Core.Status.Edition == "" {
		h.c.Capabilities.Core.Status.Edition = "community" // TODO make build determined
	}
	if h.c.Capabilities.Core.Status.ProductName == "" {
		h.c.Capabilities.Core.Status.ProductName = "reva" // TODO make build determined
	}
	if h.c.Capabilities.Core.Status.Hostname == "" {
		h.c.Capabilities.Core.Status.Hostname = "" // TODO get from context?
	}

	// checksums

	if h.c.Capabilities.Checksums == nil {
		h.c.Capabilities.Checksums = &CapabilitiesChecksums{}
	}
	if h.c.Capabilities.Checksums.SupportedTypes == nil {
		h.c.Capabilities.Checksums.SupportedTypes = []string{"SHA256"}
	}
	if h.c.Capabilities.Checksums.PreferredUploadType == "" {
		h.c.Capabilities.Checksums.PreferredUploadType = "SHA1"
	}

	// files

	if h.c.Capabilities.Files == nil {
		h.c.Capabilities.Files = &CapabilitiesFiles{}
	}

	// h.c.Capabilities.Files.PrivateLinks is boolean
	// h.c.Capabilities.Files.BigFileChunking is boolean  // TODO is this old or new chunking? jfd: I guess old

	if h.c.Capabilities.Files.BlacklistedFiles == nil {
		h.c.Capabilities.Files.BlacklistedFiles = []string{}
	}
	// h.c.Capabilities.Files.Undelete is boolean
	// h.c.Capabilities.Files.Versioning is boolean

	// dav

	if h.c.Capabilities.Dav == nil {
		h.c.Capabilities.Dav = &CapabilitiesDav{}
	}
	if h.c.Capabilities.Dav.Chunking == "" {
		h.c.Capabilities.Dav.Chunking = "1.0"
	}
	if h.c.Capabilities.Dav.Trashbin == "" {
		h.c.Capabilities.Dav.Trashbin = "1.0"
	}
	if h.c.Capabilities.Dav.Reports == nil {
		h.c.Capabilities.Dav.Reports = []string{"search-files"}
	}

	// sharing

	if h.c.Capabilities.FilesSharing == nil {
		h.c.Capabilities.FilesSharing = &CapabilitiesFilesSharing{}
	}

	// h.c.Capabilities.FilesSharing.APIEnabled is boolean

	if h.c.Capabilities.FilesSharing.Public == nil {
		h.c.Capabilities.FilesSharing.Public = &CapabilitiesFilesSharingPublic{}
	}

	// h.c.Capabilities.FilesSharing.Public.Enabled is boolean

	if h.c.Capabilities.FilesSharing.Public.Password == nil {
		h.c.Capabilities.FilesSharing.Public.Password = &CapabilitiesFilesSharingPublicPassword{}
	}

	if h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor == nil {
		h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor = &CapabilitiesFilesSharingPublicPasswordEnforcedFor{}
	}

	// h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor.ReadOnly is boolean
	// h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor.ReadWrite is boolean
	// h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor.UploadOnly is boolean

	// h.c.Capabilities.FilesSharing.Public.Password.Enforced is boolean

	if h.c.Capabilities.FilesSharing.Public.ExpireDate == nil {
		h.c.Capabilities.FilesSharing.Public.ExpireDate = &CapabilitiesFilesSharingPublicExpireDate{}
	}
	// h.c.Capabilities.FilesSharing.Public.ExpireDate.Enabled is boolean

	// h.c.Capabilities.FilesSharing.Public.SendMail is boolean
	// h.c.Capabilities.FilesSharing.Public.SocialShare is boolean
	// h.c.Capabilities.FilesSharing.Public.Upload is boolean
	// h.c.Capabilities.FilesSharing.Public.Multiple is boolean
	// h.c.Capabilities.FilesSharing.Public.SupportsUploadOnly is boolean

	if h.c.Capabilities.FilesSharing.User == nil {
		h.c.Capabilities.FilesSharing.User = &CapabilitiesFilesSharingUser{}
	}

	// h.c.Capabilities.FilesSharing.User.SendMail is boolean

	// h.c.Capabilities.FilesSharing.Resharing is boolean
	// h.c.Capabilities.FilesSharing.GroupSharing is boolean
	// h.c.Capabilities.FilesSharing.AutoAcceptShare is boolean
	// h.c.Capabilities.FilesSharing.ShareWithGroupMembersOnly is boolean
	// h.c.Capabilities.FilesSharing.ShareWithMembershipGroupsOnly is boolean

	if h.c.Capabilities.FilesSharing.UserEnumeration == nil {
		h.c.Capabilities.FilesSharing.UserEnumeration = &CapabilitiesFilesSharingUserEnumeration{}
	}

	// h.c.Capabilities.FilesSharing.UserEnumeration.Enabled is boolean
	// h.c.Capabilities.FilesSharing.UserEnumeration.GroupMembersOnly is boolean

	if h.c.Capabilities.FilesSharing.DefaultPermissions == 0 {
		h.c.Capabilities.FilesSharing.DefaultPermissions = 31
	}
	if h.c.Capabilities.FilesSharing.Federation == nil {
		h.c.Capabilities.FilesSharing.Federation = &CapabilitiesFilesSharingFederation{}
	}

	// h.c.Capabilities.FilesSharing.Federation.Outgoing is boolean
	// h.c.Capabilities.FilesSharing.Federation.Incoming is boolean

	if h.c.Capabilities.FilesSharing.SearchMinLength == 0 {
		h.c.Capabilities.FilesSharing.SearchMinLength = 2
	}

	// notifications

	if h.c.Capabilities.Notifications == nil {
		h.c.Capabilities.Notifications = &CapabilitiesNotifications{}
	}
	if h.c.Capabilities.Notifications.Endpoints == nil {
		h.c.Capabilities.Notifications.Endpoints = []string{"list", "get", "delete"}
	}

	// version

	if h.c.Version == nil {
		h.c.Version = &Version{
			// TODO get from build env
			Major:   10,
			Minor:   0,
			Micro:   9,
			String:  "10.0.9",
			Edition: "community",
		}
	}

}

// Handler renders the capabilities
func (h *CapabilitiesHandler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteOCSSuccess(w, r, h.c)
	})
}

// CapabilitiesData TODO document
type CapabilitiesData struct {
	Capabilities *Capabilities `json:"capabilities" xml:"capabilities"`
	Version      *Version      `json:"version" xml:"version"`
}

// Capabilities groups several capability aspects
type Capabilities struct {
	Core          *CapabilitiesCore          `json:"core" xml:"core"`
	Checksums     *CapabilitiesChecksums     `json:"checksums" xml:"checksums"`
	Files         *CapabilitiesFiles         `json:"files" xml:"files"`
	Dav           *CapabilitiesDav           `json:"dav" xml:"dav"`
	FilesSharing  *CapabilitiesFilesSharing  `json:"files_sharing" xml:"files_sharing" mapstructure:"files_sharing"`
	Notifications *CapabilitiesNotifications `json:"notifications" xml:"notifications"`
}

// CapabilitiesCore holds webdav config
type CapabilitiesCore struct {
	PollInterval int     `json:"pollinterval" xml:"pollinterval" mapstructure:"poll_interval"`
	WebdavRoot   string  `json:"webdav-root,omitempty" xml:"webdav-root,omitempty" mapstructure:"webdav_root"`
	Status       *Status `json:"status" xml:"status"`
}

// Status holds basic status information
type Status struct {
	Installed      ocsBool `json:"installed" xml:"installed"`
	Maintenance    ocsBool `json:"maintenance" xml:"maintenance"`
	NeedsDBUpgrade ocsBool `json:"needsDbUpgrade" xml:"needsDbUpgrade"`
	Version        string  `json:"version" xml:"version"`
	VersionString  string  `json:"versionstring" xml:"versionstring"`
	Edition        string  `json:"edition" xml:"edition"`
	ProductName    string  `json:"productname" xml:"productname"`
	Hostname       string  `json:"hostname,omitempty" xml:"hostname,omitempty"`
}

// CapabilitiesChecksums holds available hashes
type CapabilitiesChecksums struct {
	SupportedTypes      []string `json:"supportedTypes" xml:"supportedTypes>element" mapstructure:"supported_types"`
	PreferredUploadType string   `json:"preferredUploadType" xml:"preferredUploadType" mapstructure:"preferred_upload_type"`
}

// CapabilitiesFiles TODO this is storage specific, not global. What effect do these options have on the clients?
type CapabilitiesFiles struct {
	PrivateLinks     ocsBool  `json:"privateLinks" xml:"privateLinks" mapstructure:"private_links"`
	BigFileChunking  ocsBool  `json:"bigfilechunking" xml:"bigfilechunking"`
	Undelete         ocsBool  `json:"undelete" xml:"undelete"`
	Versioning       ocsBool  `json:"versioning" xml:"versioning"`
	BlacklistedFiles []string `json:"blacklisted_files" xml:"blacklisted_files>element" mapstructure:"blacklisted_files"`
}

// CapabilitiesDav holds dav endpoint config
type CapabilitiesDav struct {
	Chunking string   `json:"chunking" xml:"chunking"`
	Trashbin string   `json:"trashbin" xml:"trashbin"`
	Reports  []string `json:"reports" xml:"reports>element" mapstructure:"reports"`
}

// CapabilitiesFilesSharing TODO document
type CapabilitiesFilesSharing struct {
	APIEnabled                    ocsBool                                  `json:"api_enabled" xml:"api_enabled" mapstructure:"api_enabled"`
	Resharing                     ocsBool                                  `json:"resharing" xml:"resharing"`
	GroupSharing                  ocsBool                                  `json:"group_sharing" xml:"group_sharing" mapstructure:"group_sharing"`
	AutoAcceptShare               ocsBool                                  `json:"auto_accept_share" xml:"auto_accept_share" mapstructure:"auto_accept_share"`
	ShareWithGroupMembersOnly     ocsBool                                  `json:"share_with_group_members_only" xml:"share_with_group_members_only" mapstructure:"share_with_group_members_only"`
	ShareWithMembershipGroupsOnly ocsBool                                  `json:"share_with_membership_groups_only" xml:"share_with_membership_groups_only" mapstructure:"share_with_membership_groups_only"`
	SearchMinLength               int                                      `json:"search_min_length" xml:"search_min_length" mapstructure:"search_min_length"`
	DefaultPermissions            int                                      `json:"default_permissions" xml:"default_permissions" mapstructure:"default_permissions"`
	UserEnumeration               *CapabilitiesFilesSharingUserEnumeration `json:"user_enumeration" xml:"user_enumeration" mapstructure:"user_enumeration"`
	Federation                    *CapabilitiesFilesSharingFederation      `json:"federation" xml:"federation"`
	Public                        *CapabilitiesFilesSharingPublic          `json:"public" xml:"public"`
	User                          *CapabilitiesFilesSharingUser            `json:"user" xml:"user"`
}

// CapabilitiesFilesSharingPublic TODO document
type CapabilitiesFilesSharingPublic struct {
	Enabled            ocsBool                                   `json:"enabled" xml:"enabled"`
	SendMail           ocsBool                                   `json:"send_mail" xml:"send_mail" mapstructure:"send_mail"`
	SocialShare        ocsBool                                   `json:"social_share" xml:"social_share" mapstructure:"social_share"`
	Upload             ocsBool                                   `json:"upload" xml:"upload"`
	Multiple           ocsBool                                   `json:"multiple" xml:"multiple"`
	SupportsUploadOnly ocsBool                                   `json:"supports_upload_only" xml:"supports_upload_only" mapstructure:"supports_upload_only"`
	Password           *CapabilitiesFilesSharingPublicPassword   `json:"password" xml:"password"`
	ExpireDate         *CapabilitiesFilesSharingPublicExpireDate `json:"expire_date" xml:"expire_date" mapstructure:"expire_date"`
}

// CapabilitiesFilesSharingPublicPassword TODO document
type CapabilitiesFilesSharingPublicPassword struct {
	EnforcedFor *CapabilitiesFilesSharingPublicPasswordEnforcedFor `json:"enforced_for" xml:"enforced_for" mapstructure:"enforced_for"`
	Enforced    ocsBool                                            `json:"enforced" xml:"enforced"`
}

// CapabilitiesFilesSharingPublicPasswordEnforcedFor TODO document
type CapabilitiesFilesSharingPublicPasswordEnforcedFor struct {
	ReadOnly   ocsBool `json:"read_only" xml:"read_only,omitempty" mapstructure:"read_only"`
	ReadWrite  ocsBool `json:"read_write" xml:"read_write,omitempty" mapstructure:"read_write"`
	UploadOnly ocsBool `json:"upload_only" xml:"upload_only,omitempty" mapstructure:"upload_only"`
}

// CapabilitiesFilesSharingPublicExpireDate TODO document
type CapabilitiesFilesSharingPublicExpireDate struct {
	Enabled ocsBool `json:"enabled" xml:"enabled"`
}

// CapabilitiesFilesSharingUser TODO document
type CapabilitiesFilesSharingUser struct {
	SendMail ocsBool `json:"send_mail" xml:"send_mail" mapstructure:"send_mail"`
}

// CapabilitiesFilesSharingUserEnumeration TODO document
type CapabilitiesFilesSharingUserEnumeration struct {
	Enabled          ocsBool `json:"enabled" xml:"enabled"`
	GroupMembersOnly ocsBool `json:"group_members_only" xml:"group_members_only" mapstructure:"group_members_only"`
}

// CapabilitiesFilesSharingFederation holds outgoing and incoming flags
type CapabilitiesFilesSharingFederation struct {
	Outgoing ocsBool `json:"outgoing" xml:"outgoing"`
	Incoming ocsBool `json:"incoming" xml:"incoming"`
}

// CapabilitiesNotifications holds a list of notification endpoints
type CapabilitiesNotifications struct {
	Endpoints []string `json:"ocs-endpoints" xml:"ocs-endpoints>element" mapstructure:"endpoints"`
}

// Version holds version information
type Version struct {
	Major   int    `json:"major" xml:"major"`
	Minor   int    `json:"minor" xml:"minor"`
	Micro   int    `json:"micro" xml:"micro"` // = patch level
	String  string `json:"string" xml:"string"`
	Edition string `json:"edition" xml:"edition"`
}
