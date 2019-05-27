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

package ocssvc

import (
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
)

type CapabilitiesHandler struct {
	c CapabilitiesData
}

func (h *CapabilitiesHandler) init(c *Config) {
	h.c = c.Capabilities
}

func (h *CapabilitiesHandler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := &Response{
			OCS: &Payload{
				Meta: MetaOK,
				Data: h.c,
			},
		}

		err := WriteOCSResponse(w, r, res)
		if err != nil {
			appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing ocs response")
			w.WriteHeader(http.StatusInternalServerError)
		}
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
	Installed      bool   `json:"installed" xml:"installed"`
	Maintenance    bool   `json:"maintenance" xml:"maintenance"`
	NeedsDBUpgrade bool   `json:"needsDbUpgrade" xml:"needsDbUpgrade"`
	Version        string `json:"version" xml:"version"`
	VersionString  string `json:"versionstring" xml:"versionstring"`
	Edition        string `json:"edition" xml:"edition"`
	ProductName    string `json:"productname" xml:"productname"`
	Hostname       string `json:"hostname,omitempty" xml:"hostname,omitempty"`
}

// CapabilitiesChecksums holds available hashes
type CapabilitiesChecksums struct {
	SupportedTypes      []string `json:"supportedTypes" xml:"supportedTypes" mapstructure:"supported_types"`
	PreferredUploadType string   `json:"preferredUploadType" xml:"preferredUploadType" mapstructure:"preferred_upload_type"`
}

// CapabilitiesFiles TODO this is storage specific, not global. What effect do these options have on the clients?
type CapabilitiesFiles struct {
	PrivateLinks     bool     `json:"privateLinks" xml:"privateLinks" mapstructure:"private_links"`
	BigFileChunking  bool     `json:"bigfilechunking" xml:"bigfilechunking"`
	BlacklistedFiles []string `json:"blacklisted_files" xml:"blacklisted_files" mapstructure:"blacklisted_files"`
	Undelete         bool     `json:"undelete" xml:"undelete"`
	Versioning       bool     `json:"versioning" xml:"versioning"`
}

// CapabilitiesDav holds the chunking version
type CapabilitiesDav struct {
	Chunking string `json:"chunking" xml:"chunking"`
}

// CapabilitiesFilesSharing TODO document
type CapabilitiesFilesSharing struct {
	APIEnabled                    bool                                     `json:"api_enabled" xml:"api_enabled" mapstructure:"api_enabled"`
	Public                        *CapabilitiesFilesSharingPublic          `json:"public" xml:"public"`
	User                          *CapabilitiesFilesSharingUser            `json:"user" xml:"user"`
	Resharing                     bool                                     `json:"resharing" xml:"resharing"`
	GroupSharing                  bool                                     `json:"group_sharing" xml:"group_sharing" mapstructure:"group_sharing"`
	AutoAcceptShare               bool                                     `json:"auto_accept_share" xml:"auto_accept_share" mapstructure:"auto_accept_share"`
	ShareWithGroupMembersOnly     bool                                     `json:"share_with_group_members_only" xml:"share_with_group_members_only" mapstructure:"share_with_group_members_only"`
	ShareWithMembershipGroupsOnly bool                                     `json:"share_with_membership_groups_only" xml:"share_with_membership_groups_only" mapstructure:"share_with_membership_groups_only"`
	UserEnumeration               *CapabilitiesFilesSharingUserEnumeration `json:"user_enumeration" xml:"user_enumeration" mapstructure:"user_enumeration"`
	DefaultPermissions            int                                      `json:"default_permissions" xml:"default_permissions" mapstructure:"default_permissions"`
	Federation                    *CapabilitiesFilesSharingFederation      `json:"federation" xml:"federation"`
	SearchMinLength               int                                      `json:"search_min_length" xml:"search_min_length" mapstructure:"search_min_length"`
}

// CapabilitiesFilesSharingPublic TODO document
type CapabilitiesFilesSharingPublic struct {
	Enabled            bool                                      `json:"enabled" xml:"enabled"`
	Password           *CapabilitiesFilesSharingPublicPassword   `json:"password" xml:"password"`
	ExpireDate         *CapabilitiesFilesSharingPublicExpireDate `json:"expire_date" xml:"expire_date" mapstructure:"expire_date"`
	SendMail           bool                                      `json:"send_mail" xml:"send_mail" mapstructure:"send_mail"`
	SocialShare        bool                                      `json:"social_share" xml:"social_share" mapstructure:"social_share"`
	Upload             bool                                      `json:"upload" xml:"upload"`
	Multiple           bool                                      `json:"multiple" xml:"multiple"`
	SupportsUploadOnly bool                                      `json:"supports_upload_only" xml:"supports_upload_only" mapstructure:"supports_upload_only"`
}

// CapabilitiesFilesSharingPublicPassword TODO document
type CapabilitiesFilesSharingPublicPassword struct {
	EnforcedFor *CapabilitiesFilesSharingPublicPasswordEnforcedFor `json:"enforced_for" xml:"enforced_for" mapstructure:"enforced_for"`
	Enforced    bool                                               `json:"enforced" xml:"enforced"`
}

// CapabilitiesFilesSharingPublicPasswordEnforcedFor TODO document
type CapabilitiesFilesSharingPublicPasswordEnforcedFor struct {
	ReadOnly   bool `json:"read_only" xml:"read_only" mapstructure:"read_only"`
	ReadWrite  bool `json:"read_write" xml:"read_write" mapstructure:"read_write"`
	UploadOnly bool `json:"upload_only" xml:"upload_only" mapstructure:"upload_only"`
}

// CapabilitiesFilesSharingPublicExpireDate TODO document
type CapabilitiesFilesSharingPublicExpireDate struct {
	Enabled bool `json:"enabled" xml:"enabled"`
}

// CapabilitiesFilesSharingUser TODO document
type CapabilitiesFilesSharingUser struct {
	SendMail bool `json:"send_mail" xml:"send_mail" mapstructure:"send_mail"`
}

// CapabilitiesFilesSharingUserEnumeration TODO document
type CapabilitiesFilesSharingUserEnumeration struct {
	Enabled          bool `json:"enabled" xml:"enabled"`
	GroupMembersOnly bool `json:"group_members_only" xml:"group_members_only" mapstructure:"group_members_only"`
}

// CapabilitiesFilesSharingFederation holds outgoing and incoming flags
type CapabilitiesFilesSharingFederation struct {
	Outgoing bool `json:"outgoing" xml:"outgoing"`
	Incoming bool `json:"incoming" xml:"incoming"`
}

// CapabilitiesNotifications holds a list of notification endpoints
type CapabilitiesNotifications struct {
	Endpoints []string `json:"ocs-endpoints" xml:"ocs-endpoints" mapstructure:"endpoints"`
}

// Version holds version information
type Version struct {
	Major   int    `json:"major" xml:"major"`
	Minor   int    `json:"minor" xml:"minor"`
	Micro   int    `json:"micro" xml:"micro"` // = patch level
	String  string `json:"string" xml:"string"`
	Edition string `json:"edition" xml:"edition"`
}
