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

package capabilities

import (
	"net/http"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/data"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
)

// Handler renders the capability endpoint
type Handler struct {
	c                     data.CapabilitiesData
	defaultUploadProtocol string
	userAgentChunkingMap  map[string]string
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) {
	h.c = c.Capabilities
	h.defaultUploadProtocol = c.DefaultUploadProtocol
	h.userAgentChunkingMap = c.UserAgentChunkingMap

	// capabilities
	if h.c.Capabilities == nil {
		h.c.Capabilities = &data.Capabilities{}
	}

	// core

	if h.c.Capabilities.Core == nil {
		h.c.Capabilities.Core = &data.CapabilitiesCore{}
	}
	if h.c.Capabilities.Core.PollInterval == 0 {
		h.c.Capabilities.Core.PollInterval = 60
	}
	if h.c.Capabilities.Core.WebdavRoot == "" {
		h.c.Capabilities.Core.WebdavRoot = "remote.php/webdav"
	}
	// h.c.Capabilities.Core.SupportURLSigning is boolean

	if h.c.Capabilities.Core.Status == nil {
		h.c.Capabilities.Core.Status = &data.Status{}
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
		h.c.Capabilities.Checksums = &data.CapabilitiesChecksums{}
	}
	if h.c.Capabilities.Checksums.SupportedTypes == nil {
		h.c.Capabilities.Checksums.SupportedTypes = []string{"SHA256"}
	}
	if h.c.Capabilities.Checksums.PreferredUploadType == "" {
		h.c.Capabilities.Checksums.PreferredUploadType = "SHA1"
	}

	// files

	if h.c.Capabilities.Files == nil {
		h.c.Capabilities.Files = &data.CapabilitiesFiles{}
	}

	if h.c.Capabilities.Files.BlacklistedFiles == nil {
		h.c.Capabilities.Files.BlacklistedFiles = []string{}
	}
	// h.c.Capabilities.Files.Undelete is boolean
	// h.c.Capabilities.Files.Versioning is boolean
	// h.c.Capabilities.Files.Favorites is boolean

	// dav

	if h.c.Capabilities.Dav == nil {
		h.c.Capabilities.Dav = &data.CapabilitiesDav{}
	}
	if h.c.Capabilities.Dav.Trashbin == "" {
		h.c.Capabilities.Dav.Trashbin = "1.0"
	}
	if h.c.Capabilities.Dav.Reports == nil {
		h.c.Capabilities.Dav.Reports = []string{"search-files"}
	}

	// sharing

	if h.c.Capabilities.FilesSharing == nil {
		h.c.Capabilities.FilesSharing = &data.CapabilitiesFilesSharing{}
	}

	// h.c.Capabilities.FilesSharing.APIEnabled is boolean

	if h.c.Capabilities.FilesSharing.Public == nil {
		h.c.Capabilities.FilesSharing.Public = &data.CapabilitiesFilesSharingPublic{}
	}

	// h.c.Capabilities.FilesSharing.Public.Enabled is boolean
	h.c.Capabilities.FilesSharing.Public.Enabled = true

	if h.c.Capabilities.FilesSharing.Public.Password == nil {
		h.c.Capabilities.FilesSharing.Public.Password = &data.CapabilitiesFilesSharingPublicPassword{}
	}

	if h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor == nil {
		h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor = &data.CapabilitiesFilesSharingPublicPasswordEnforcedFor{}
	}

	// h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor.ReadOnly is boolean
	// h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor.ReadWrite is boolean
	// h.c.Capabilities.FilesSharing.Public.Password.EnforcedFor.UploadOnly is boolean

	// h.c.Capabilities.FilesSharing.Public.Password.Enforced is boolean

	if h.c.Capabilities.FilesSharing.Public.ExpireDate == nil {
		h.c.Capabilities.FilesSharing.Public.ExpireDate = &data.CapabilitiesFilesSharingPublicExpireDate{}
	}
	// h.c.Capabilities.FilesSharing.Public.ExpireDate.Enabled is boolean

	// h.c.Capabilities.FilesSharing.Public.SendMail is boolean
	// h.c.Capabilities.FilesSharing.Public.SocialShare is boolean
	// h.c.Capabilities.FilesSharing.Public.Upload is boolean
	// h.c.Capabilities.FilesSharing.Public.Multiple is boolean
	// h.c.Capabilities.FilesSharing.Public.SupportsUploadOnly is boolean

	if h.c.Capabilities.FilesSharing.User == nil {
		h.c.Capabilities.FilesSharing.User = &data.CapabilitiesFilesSharingUser{}
	}

	// h.c.Capabilities.FilesSharing.User.SendMail is boolean

	// h.c.Capabilities.FilesSharing.Resharing is boolean
	// h.c.Capabilities.FilesSharing.GroupSharing is boolean
	// h.c.Capabilities.FilesSharing.AutoAcceptShare is boolean
	// h.c.Capabilities.FilesSharing.ShareWithGroupMembersOnly is boolean
	// h.c.Capabilities.FilesSharing.ShareWithMembershipGroupsOnly is boolean

	if h.c.Capabilities.FilesSharing.UserEnumeration == nil {
		h.c.Capabilities.FilesSharing.UserEnumeration = &data.CapabilitiesFilesSharingUserEnumeration{}
	}

	// h.c.Capabilities.FilesSharing.UserEnumeration.Enabled is boolean
	// h.c.Capabilities.FilesSharing.UserEnumeration.GroupMembersOnly is boolean

	if h.c.Capabilities.FilesSharing.DefaultPermissions == 0 {
		h.c.Capabilities.FilesSharing.DefaultPermissions = 31
	}
	if h.c.Capabilities.FilesSharing.Federation == nil {
		h.c.Capabilities.FilesSharing.Federation = &data.CapabilitiesFilesSharingFederation{}
	}

	// h.c.Capabilities.FilesSharing.Federation.Outgoing is boolean
	// h.c.Capabilities.FilesSharing.Federation.Incoming is boolean

	if h.c.Capabilities.FilesSharing.SearchMinLength == 0 {
		h.c.Capabilities.FilesSharing.SearchMinLength = 2
	}

	// notifications

	if h.c.Capabilities.Notifications == nil {
		h.c.Capabilities.Notifications = &data.CapabilitiesNotifications{}
	}
	if h.c.Capabilities.Notifications.Endpoints == nil {
		h.c.Capabilities.Notifications.Endpoints = []string{"list", "get", "delete"}
	}

	// version

	if h.c.Version == nil {
		h.c.Version = &data.Version{
			// TODO get from build env
			Major:   10,
			Minor:   0,
			Micro:   9,
			String:  "10.0.9",
			Edition: "community",
		}
	}

	// upload protocol-specific details
	setCapabilitiesForChunkProtocol(chunkProtocol(h.defaultUploadProtocol), &h.c)

}

// Handler renders the capabilities
func (h *Handler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := h.getCapabilitiesForUserAgent(r.UserAgent())
		response.WriteOCSSuccess(w, r, c)
	})
}
