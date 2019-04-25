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

package ocdavsvc

import (
	"encoding/json"
	"encoding/xml"
	"net/http"

	"github.com/cernbox/reva/pkg/appctx"
)

type ocsResponse struct {
	OCS *ocsPayload `json:"ocs"`
}

type ocsPayload struct {
	XMLName struct{}         `json:"-" xml:"ocs"`
	Meta    *ocsResponseMeta `json:"meta" xml:"meta"`
	Data    interface{}      `json:"data,omitempty" xml:"data,omitempty"`
}

type ocsResponseMeta struct {
	Status       string `json:"status" xml:"status"`
	StatusCode   int    `json:"statuscode" xml:"statuscode"`
	Message      string `json:"message" xml:"message"`
	TotalItems   string `json:"totalitems,omitempty" xml:"totalitems,omitempty"`
	ItemsPerPage string `json:"itemsperpage,omitempty" xml:"itemsperpage,omitempty"`
}

var ocsMetaOK = &ocsResponseMeta{Status: "ok", StatusCode: 100, Message: "OK"}

type ocsUserData struct {
	// TODO needs better naming, clarify if we need a userid, a username or both
	ID          string `json:"id" xml:"id"`
	DisplayName string `json:"display-name" xml:"display-name"`
	Email       string `json:"email" xml:"email"`
}

type ocsConfigData struct {
	Version string `json:"version" xml:"version"`
	Website string `json:"website" xml:"website"`
	Host    string `json:"host" xml:"host"`
	Contact string `json:"contact" xml:"contact"`
	SSL     string `json:"ssl" xml:"ssl"`
}

type ocsCapabilitiesData struct {
	Capabilities *ocsCapabilities `json:"capabilities" xml:"capabilities"`
	Version      *ocsVersion      `json:"version" xml:"version"`
}

type ocsCapabilities struct {
	Core          *ocsCapabilitiesCore          `json:"core" xml:"core"`
	Checksums     *ocsCapabilitiesChecksums     `json:"checksums" xml:"checksums"`
	Files         *ocsCapabilitiesFiles         `json:"files" xml:"files"`
	Dav           *ocsCapabilitiesDav           `json:"dav" xml:"dav"`
	FilesSharing  *ocsCapabilitiesFilesSharing  `json:"files_sharing" xml:"files_sharing"`
	Notifications *ocsCapabilitiesNotifications `json:"notifications" xml:"notifications"`
}
type ocsCapabilitiesCore struct {
	PollInterval int        `json:"pollinterval" xml:"pollinterval"`
	WebdavRoot   string     `json:"webdav-root,omitempty" xml:"webdav-root,omitempty"`
	Status       *ocsStatus `json:"status" xml:"status"`
}
type ocsStatus struct {
	Installed      bool   `json:"installed" xml:"installed"`
	Maintenance    bool   `json:"maintenance" xml:"maintenance"`
	NeedsDBUpgrade bool   `json:"needsDbUpgrade" xml:"needsDbUpgrade"`
	Version        string `json:"version" xml:"version"`
	VersionString  string `json:"versionstring" xml:"versionstring"`
	Edition        string `json:"edition" xml:"edition"`
	ProductName    string `json:"productname" xml:"productname"`
	Hostname       string `json:"hostname,omitempty" xml:"hostname,omitempty"`
}

type ocsCapabilitiesChecksums struct {
	SupportedTypes      []string `json:"supportedTypes" xml:"supportedTypes"`
	PreferredUploadType string   `json:"preferredUploadType" xml:"preferredUploadType"`
}

// TODO this is storage specific, not global. What effect do these options have on the clients?
type ocsCapabilitiesFiles struct {
	PrivateLinks     bool     `json:"privateLinks" xml:"privateLinks"`
	BigFileChunking  bool     `json:"bigfilechunking" xml:"bigfilechunking"`
	BlacklistedFiles []string `json:"blacklisted_files" xml:"blacklisted_files"`
	Undelete         bool     `json:"undelete" xml:"undelete"`
	Versioning       bool     `json:"versioning" xml:"versioning"`
}

type ocsCapabilitiesDav struct {
	Chunking string `json:"chunking" xml:"chunking"`
}
type ocsCapabilitiesFilesSharing struct {
	APIEnabled                    bool                                        `json:"api_enabled" xml:"api_enabled"`
	Public                        *ocsCapabilitiesFilesSharingPublic          `json:"public" xml:"public"`
	User                          *ocsCapabilitiesFilesSharingUser            `json:"user" xml:"user"`
	Resharing                     bool                                        `json:"resharing" xml:"resharing"`
	GroupSharing                  bool                                        `json:"group_sharing" xml:"group_sharing"`
	AutoAcceptShare               bool                                        `json:"auto_accept_share" xml:"auto_accept_share"`
	ShareWithGroupMembersOnly     bool                                        `json:"share_with_group_members_only" xml:"share_with_group_members_only"`
	ShareWithMembershipGroupsOnly bool                                        `json:"share_with_membership_groups_only" xml:"share_with_membership_groups_only"`
	UserEnumeration               *ocsCapabilitiesFilesSharingUserEnumeration `json:"user_enumeration" xml:"user_enumeration"`
	DefaultPermissions            int                                         `json:"default_permissions" xml:"default_permissions"`
	Federation                    *ocsCapabilitiesFilesSharingFederation      `json:"federation" xml:"federation"`
	SearchMinLength               int                                         `json:"search_min_length" xml:"search_min_length"`
}

type ocsCapabilitiesFilesSharingPublic struct {
	Enabled            bool                                         `json:"enabled" xml:"enabled"`
	Password           *ocsCapabilitiesFilesSharingPublicPassword   `json:"password" xml:"password"`
	ExpireDate         *ocsCapabilitiesFilesSharingPublicExpireDate `json:"expire_date" xml:"expire_date"`
	SendMail           bool                                         `json:"send_mail" xml:"send_mail"`
	SocialShare        bool                                         `json:"social_share" xml:"social_share"`
	Upload             bool                                         `json:"upload" xml:"upload"`
	Multiple           bool                                         `json:"multiple" xml:"multiple"`
	SupportsUploadOnly bool                                         `json:"supports_upload_only" xml:"supports_upload_only"`
}
type ocsCapabilitiesFilesSharingPublicPassword struct {
	EnforcedFor *ocsCapabilitiesFilesSharingPublicPasswordEnforcedFor `json:"enforced_for" xml:"enforced_for"`
	Enforced    bool                                                  `json:"enforced" xml:"enforced"`
}
type ocsCapabilitiesFilesSharingPublicPasswordEnforcedFor struct {
	ReadOnly   bool `json:"read_only" xml:"read_only"`
	ReadWrite  bool `json:"read_write" xml:"read_write"`
	UploadOnly bool `json:"upload_only" xml:"upload_only"`
}
type ocsCapabilitiesFilesSharingPublicExpireDate struct {
	Enabled bool `json:"enabled" xml:"enabled"`
}
type ocsCapabilitiesFilesSharingUser struct {
	SendMail bool `json:"send_mail" xml:"send_mail"`
}
type ocsCapabilitiesFilesSharingUserEnumeration struct {
	Enabled          bool `json:"enabled" xml:"enabled"`
	GroupMembersOnly bool `json:"group_members_only" xml:"group_members_only"`
}

type ocsCapabilitiesFilesSharingFederation struct {
	Outgoing bool `json:"outgoing" xml:"outgoing"`
	Incoming bool `json:"incoming" xml:"incoming"`
}

type ocsCapabilitiesNotifications struct {
	OCSEndpoints []string `json:"ocs-endpoints" xml:"ocs-endpoints"`
}

type ocsVersion struct {
	Major   int    `json:"major" xml:"major"`
	Minor   int    `json:"minor" xml:"minor"`
	Micro   int    `json:"micro" xml:"micro"` // = patch level
	String  string `json:"string" xml:"string"`
	Edition string `json:"edition" xml:"edition"`
}

// handles writing ocs responses in json and xml
func writeOCSResponse(w http.ResponseWriter, r *http.Request, res *ocsResponse) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	var encoded []byte
	var err error
	if r.URL.Query().Get("format") == "xml" {
		w.Write([]byte(xml.Header))
		encoded, err = xml.Marshal(res.OCS)
		w.Header().Set("Content-Type", "application/xml")
	} else {
		encoded, err = json.Marshal(res)
		w.Header().Set("Content-Type", "application/json")
	}
	if err != nil {
		log.Error().Err(err).Msg("error writing ocs response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(encoded)
}
