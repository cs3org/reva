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

package data

import (
	"encoding/xml"
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

// CapabilitiesData TODO document.
type CapabilitiesData struct {
	Capabilities *Capabilities `json:"capabilities" xml:"capabilities"`
	Version      *Version      `json:"version"      xml:"version"`
}

// Capabilities groups several capability aspects.
type Capabilities struct {
	Core         *CapabilitiesCore         `json:"core"             xml:"core"`
	Checksums    *CapabilitiesChecksums    `json:"checksums"        xml:"checksums"`
	Files        *CapabilitiesFiles        `json:"files"            mapstructure:"files"         xml:"files"`
	Dav          *CapabilitiesDav          `json:"dav"              xml:"dav"`
	FilesSharing *CapabilitiesFilesSharing `json:"files_sharing"    mapstructure:"files_sharing" xml:"files_sharing"`
	Spaces       *Spaces                   `json:"spaces,omitempty" mapstructure:"spaces"        xml:"spaces,omitempty"`

	Notifications *CapabilitiesNotifications `json:"notifications,omitempty" xml:"notifications,omitempty"`
}

// Spaces lets a service configure its advertised options related to Storage Spaces.
type Spaces struct {
	Version  string  `json:"version"  mapstructure:"version"  xml:"version"`
	Enabled  ocsBool `json:"enabled"  mapstructure:"enabled"  xml:"enabled"`
	Projects ocsBool `json:"projects" mapstructure:"projects" xml:"projects"`
}

// CapabilitiesCore holds webdav config.
type CapabilitiesCore struct {
	PollInterval      int     `json:"pollinterval"          mapstructure:"poll_interval"       xml:"pollinterval"`
	WebdavRoot        string  `json:"webdav-root,omitempty" mapstructure:"webdav_root"         xml:"webdav-root,omitempty"`
	Status            *Status `json:"status"                xml:"status"`
	SupportURLSigning ocsBool `json:"support-url-signing"   mapstructure:"support_url_signing" xml:"support-url-signing"`
}

// Status holds basic status information.
type Status struct {
	Installed      ocsBool `json:"installed"          xml:"installed"`
	Maintenance    ocsBool `json:"maintenance"        xml:"maintenance"`
	NeedsDBUpgrade ocsBool `json:"needsDbUpgrade"     xml:"needsDbUpgrade"`
	Version        string  `json:"version"            xml:"version"`
	VersionString  string  `json:"versionstring"      xml:"versionstring"`
	Edition        string  `json:"edition"            xml:"edition"`
	ProductName    string  `json:"productname"        xml:"productname"`
	Product        string  `json:"product"            xml:"product"`
	Hostname       string  `json:"hostname,omitempty" xml:"hostname,omitempty"`
}

// CapabilitiesChecksums holds available hashes.
type CapabilitiesChecksums struct {
	SupportedTypes      []string `json:"supportedTypes"      mapstructure:"supported_types"       xml:"supportedTypes>element"`
	PreferredUploadType string   `json:"preferredUploadType" mapstructure:"preferred_upload_type" xml:"preferredUploadType"`
}

// CapabilitiesFilesTusSupport TODO this must be a summary of storages.
type CapabilitiesFilesTusSupport struct {
	Version            string `json:"version"              xml:"version"`
	Resumable          string `json:"resumable"            xml:"resumable"`
	Extension          string `json:"extension"            xml:"extension"`
	MaxChunkSize       int    `json:"max_chunk_size"       mapstructure:"max_chunk_size"       xml:"max_chunk_size"`
	HTTPMethodOverride string `json:"http_method_override" mapstructure:"http_method_override" xml:"http_method_override"`
}

// CapabilitiesArchiver holds available archivers information.
type CapabilitiesArchiver struct {
	Enabled     bool     `json:"enabled"       mapstructure:"enabled"       xml:"enabled"`
	Version     string   `json:"version"       mapstructure:"version"       xml:"version"`
	Formats     []string `json:"formats"       mapstructure:"formats"       xml:"formats"`
	ArchiverURL string   `json:"archiver_url"  mapstructure:"archiver_url"  xml:"archiver_url"`
	MaxNumFiles string   `json:"max_num_files" mapstructure:"max_num_files" xml:"max_num_files"`
	MaxSize     string   `json:"max_size"      mapstructure:"max_size"      xml:"max_size"`
}

// CapabilitiesAppProvider holds available app provider information.
type CapabilitiesAppProvider struct {
	Enabled bool   `json:"enabled"  mapstructure:"enabled"  xml:"enabled"`
	Version string `json:"version"  mapstructure:"version"  xml:"version"`
	AppsURL string `json:"apps_url" mapstructure:"apps_url" xml:"apps_url"`
	OpenURL string `json:"open_url" mapstructure:"open_url" xml:"open_url"`
	NewURL  string `json:"new_url"  mapstructure:"new_url"  xml:"new_url"`
}

// CapabilitiesFiles TODO this is storage specific, not global. What effect do these options have on the clients?
type CapabilitiesFiles struct {
	PrivateLinks      ocsBool                      `json:"privateLinks"       mapstructure:"private_links"     xml:"privateLinks"`
	BigFileChunking   ocsBool                      `json:"bigfilechunking"    xml:"bigfilechunking"`
	Undelete          ocsBool                      `json:"undelete"           xml:"undelete"`
	Versioning        ocsBool                      `json:"versioning"         xml:"versioning"`
	Favorites         ocsBool                      `json:"favorites"          xml:"favorites"`
	PermanentDeletion ocsBool                      `json:"permanent_deletion" xml:"permanent_deletion"`
	BlacklistedFiles  []string                     `json:"blacklisted_files"  mapstructure:"blacklisted_files" xml:"blacklisted_files>element"`
	TusSupport        *CapabilitiesFilesTusSupport `json:"tus_support"        mapstructure:"tus_support"       xml:"tus_support"`
	Archivers         []*CapabilitiesArchiver      `json:"archivers"          mapstructure:"archivers"         xml:"archivers"`
	AppProviders      []*CapabilitiesAppProvider   `json:"app_providers"      mapstructure:"app_providers"     xml:"app_providers"`
}

// CapabilitiesDav holds dav endpoint config.
type CapabilitiesDav struct {
	Chunking                       string   `json:"chunking"                       xml:"chunking"`
	Trashbin                       string   `json:"trashbin"                       xml:"trashbin"`
	Reports                        []string `json:"reports"                        mapstructure:"reports"               xml:"reports>element"`
	ChunkingParallelUploadDisabled bool     `json:"chunkingParallelUploadDisabled" xml:"chunkingParallelUploadDisabled"`
}

// CapabilitiesFilesSharing TODO document.
type CapabilitiesFilesSharing struct {
	APIEnabled                    ocsBool                                  `json:"api_enabled"                       mapstructure:"api_enabled"                       xml:"api_enabled"`
	Resharing                     ocsBool                                  `json:"resharing"                         mapstructure:"resharing"                         xml:"resharing"`
	ResharingDefault              ocsBool                                  `json:"resharing_default"                 mapstructure:"resharing_default"                 xml:"resharing_default"`
	DenyAccess                    ocsBool                                  `json:"deny_access"                       mapstructure:"deny_access"                       xml:"deny_access"`
	GroupSharing                  ocsBool                                  `json:"group_sharing"                     mapstructure:"group_sharing"                     xml:"group_sharing"`
	AutoAcceptShare               ocsBool                                  `json:"auto_accept_share"                 mapstructure:"auto_accept_share"                 xml:"auto_accept_share"`
	ShareWithGroupMembersOnly     ocsBool                                  `json:"share_with_group_members_only"     mapstructure:"share_with_group_members_only"     xml:"share_with_group_members_only"`
	ShareWithMembershipGroupsOnly ocsBool                                  `json:"share_with_membership_groups_only" mapstructure:"share_with_membership_groups_only" xml:"share_with_membership_groups_only"`
	CanRename                     ocsBool                                  `json:"can_rename"                        mapstructure:"can_rename"                        xml:"can_rename"`
	AllowCustom                   ocsBool                                  `json:"allow_custom"                      mapstructure:"allow_custom"                      xml:"allow_custom"`
	SearchMinLength               int                                      `json:"search_min_length"                 mapstructure:"search_min_length"                 xml:"search_min_length"`
	DefaultPermissions            int                                      `json:"default_permissions"               mapstructure:"default_permissions"               xml:"default_permissions"`
	UserEnumeration               *CapabilitiesFilesSharingUserEnumeration `json:"user_enumeration"                  mapstructure:"user_enumeration"                  xml:"user_enumeration"`
	Federation                    *CapabilitiesFilesSharingFederation      `json:"federation"                        xml:"federation"`
	Public                        *CapabilitiesFilesSharingPublic          `json:"public"                            xml:"public"`
	User                          *CapabilitiesFilesSharingUser            `json:"user"                              xml:"user"`
}

// CapabilitiesFilesSharingPublic TODO document.
type CapabilitiesFilesSharingPublic struct {
	Enabled            ocsBool                                   `json:"enabled"              xml:"enabled"`
	SendMail           ocsBool                                   `json:"send_mail"            mapstructure:"send_mail"            xml:"send_mail"`
	SocialShare        ocsBool                                   `json:"social_share"         mapstructure:"social_share"         xml:"social_share"`
	Upload             ocsBool                                   `json:"upload"               xml:"upload"`
	Multiple           ocsBool                                   `json:"multiple"             xml:"multiple"`
	SupportsUploadOnly ocsBool                                   `json:"supports_upload_only" mapstructure:"supports_upload_only" xml:"supports_upload_only"`
	CanEdit            ocsBool                                   `json:"can_edit"             mapstructure:"can_edit"             xml:"can_edit"`
	CanContribute      ocsBool                                   `json:"can_contribute"       xml:"can_contribute"`
	Password           *CapabilitiesFilesSharingPublicPassword   `json:"password"             xml:"password"`
	ExpireDate         *CapabilitiesFilesSharingPublicExpireDate `json:"expire_date"          mapstructure:"expire_date"          xml:"expire_date"`
}

// CapabilitiesFilesSharingPublicPassword TODO document.
type CapabilitiesFilesSharingPublicPassword struct {
	EnforcedFor *CapabilitiesFilesSharingPublicPasswordEnforcedFor `json:"enforced_for" mapstructure:"enforced_for" xml:"enforced_for"`
	Enforced    ocsBool                                            `json:"enforced"     xml:"enforced"`
}

// CapabilitiesFilesSharingPublicPasswordEnforcedFor TODO document.
type CapabilitiesFilesSharingPublicPasswordEnforcedFor struct {
	ReadOnly   ocsBool `json:"read_only"   mapstructure:"read_only"   xml:"read_only,omitempty"`
	ReadWrite  ocsBool `json:"read_write"  mapstructure:"read_write"  xml:"read_write,omitempty"`
	UploadOnly ocsBool `json:"upload_only" mapstructure:"upload_only" xml:"upload_only,omitempty"`
}

// CapabilitiesFilesSharingPublicExpireDate TODO document.
type CapabilitiesFilesSharingPublicExpireDate struct {
	Enabled ocsBool `json:"enabled" xml:"enabled"`
}

// CapabilitiesFilesSharingUser TODO document.
type CapabilitiesFilesSharingUser struct {
	SendMail       ocsBool                     `json:"send_mail"       mapstructure:"send_mail"       xml:"send_mail"`
	ProfilePicture ocsBool                     `json:"profile_picture" mapstructure:"profile_picture" xml:"profile_picture"`
	Settings       []*CapabilitiesUserSettings `json:"settings"        mapstructure:"settings"        xml:"settings"`
}

// CapabilitiesUserSettings holds available user settings service information.
type CapabilitiesUserSettings struct {
	Enabled bool   `json:"enabled" mapstructure:"enabled" xml:"enabled"`
	Version string `json:"version" mapstructure:"version" xml:"version"`
}

// CapabilitiesFilesSharingUserEnumeration TODO document.
type CapabilitiesFilesSharingUserEnumeration struct {
	Enabled          ocsBool `json:"enabled"            xml:"enabled"`
	GroupMembersOnly ocsBool `json:"group_members_only" mapstructure:"group_members_only" xml:"group_members_only"`
}

// CapabilitiesFilesSharingFederation holds outgoing and incoming flags.
type CapabilitiesFilesSharingFederation struct {
	Outgoing ocsBool `json:"outgoing" xml:"outgoing"`
	Incoming ocsBool `json:"incoming" xml:"incoming"`
}

// CapabilitiesNotifications holds a list of notification endpoints.
type CapabilitiesNotifications struct {
	Endpoints []string `json:"ocs-endpoints,omitempty" mapstructure:"endpoints" xml:"ocs-endpoints>element,omitempty"`
}

// Version holds version information.
type Version struct {
	Major   int    `json:"major"   xml:"major"`
	Minor   int    `json:"minor"   xml:"minor"`
	Micro   int    `json:"micro"   xml:"micro"` // = patch level
	String  string `json:"string"  xml:"string"`
	Edition string `json:"edition" xml:"edition"`
	Product string `json:"product" xml:"product"`
}
