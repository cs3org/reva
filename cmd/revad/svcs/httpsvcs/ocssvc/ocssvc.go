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
	"fmt"
	"net/http"
	"os"

	"github.com/cs3org/reva/cmd/revad/httpserver"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/mitchellh/mapstructure"
)

func init() {
	httpserver.Register("ocssvc", New)
}

type config struct {
	Prefix       string           `mapstructure:"prefix"`
	Config       ConfigData       `mapstructure:"config"`
	Capabilities CapabilitiesData `mapstructure:"capabilities"`
}

// init sets the defaults
func (c *config) init() {
	fmt.Fprintf(os.Stderr, "ocs config %+v\n", c)
	// config
	if c.Config.Version == "" {
		c.Config.Version = "1.7"
	}
	if c.Config.Website == "" {
		c.Config.Website = "reva"
	}
	if c.Config.Host == "" {
		c.Config.Host = "" // TODO get from context?
	}
	if c.Config.Contact == "" {
		c.Config.Contact = ""
	}
	if c.Config.SSL == "" {
		c.Config.SSL = "false" // TODO get from context?
	}

	// capabilities
	if c.Capabilities.Capabilities == nil {
		c.Capabilities.Capabilities = &Capabilities{}
	}

	// core

	if c.Capabilities.Capabilities.Core == nil {
		c.Capabilities.Capabilities.Core = &CapabilitiesCore{}
	}
	if c.Capabilities.Capabilities.Core.PollInterval == 0 {
		c.Capabilities.Capabilities.Core.PollInterval = 60
	}
	if c.Capabilities.Capabilities.Core.WebdavRoot == "" {
		c.Capabilities.Capabilities.Core.WebdavRoot = "remote.php/webdav"
	}

	if c.Capabilities.Capabilities.Core.Status == nil {
		c.Capabilities.Capabilities.Core.Status = &Status{}
	}
	// c.Capabilities.Capabilities.Core.Status.Installed is boolean
	// c.Capabilities.Capabilities.Core.Status.Maintenance is boolean
	// c.Capabilities.Capabilities.Core.Status.NeedsDBUpgrade is boolean
	if c.Capabilities.Capabilities.Core.Status.Version == "" {
		c.Capabilities.Capabilities.Core.Status.Version = "10.0.9.5" // TODO make build determined
	}
	if c.Capabilities.Capabilities.Core.Status.VersionString == "" {
		c.Capabilities.Capabilities.Core.Status.VersionString = "10.0.9" // TODO make build determined
	}
	if c.Capabilities.Capabilities.Core.Status.Edition == "" {
		c.Capabilities.Capabilities.Core.Status.Edition = "community" // TODO make build determined
	}
	if c.Capabilities.Capabilities.Core.Status.ProductName == "" {
		c.Capabilities.Capabilities.Core.Status.ProductName = "reva" // TODO make build determined
	}
	if c.Capabilities.Capabilities.Core.Status.Hostname == "" {
		c.Capabilities.Capabilities.Core.Status.Hostname = "" // TODO get from context?
	}

	// checksums

	if c.Capabilities.Capabilities.Checksums == nil {
		c.Capabilities.Capabilities.Checksums = &CapabilitiesChecksums{}
	}
	if c.Capabilities.Capabilities.Checksums.SupportedTypes == nil {
		c.Capabilities.Capabilities.Checksums.SupportedTypes = []string{"SHA1"}
	}
	if c.Capabilities.Capabilities.Checksums.PreferredUploadType == "" {
		c.Capabilities.Capabilities.Checksums.PreferredUploadType = "SHA1"
	}

	// files

	if c.Capabilities.Capabilities.Files == nil {
		c.Capabilities.Capabilities.Files = &CapabilitiesFiles{}
	}

	// c.Capabilities.Capabilities.Files.PrivateLinks is boolean
	// c.Capabilities.Capabilities.Files.BigFileChunking is boolean  // TODO is this old or new chunking? jfd: I guess old

	if c.Capabilities.Capabilities.Files.BlacklistedFiles == nil {
		c.Capabilities.Capabilities.Files.BlacklistedFiles = []string{}
	}
	// c.Capabilities.Capabilities.Files.Undelete is boolean
	// c.Capabilities.Capabilities.Files.Versioning is boolean

	// dav

	if c.Capabilities.Capabilities.Dav == nil {
		c.Capabilities.Capabilities.Dav = &CapabilitiesDav{}
	}
	if c.Capabilities.Capabilities.Dav.Chunking == "" {
		c.Capabilities.Capabilities.Dav.Chunking = "1.0"
	}

	// sharing

	if c.Capabilities.Capabilities.FilesSharing == nil {
		c.Capabilities.Capabilities.FilesSharing = &CapabilitiesFilesSharing{}
	}

	// c.Capabilities.Capabilities.FilesSharing.APIEnabled is boolean

	if c.Capabilities.Capabilities.FilesSharing.Public == nil {
		c.Capabilities.Capabilities.FilesSharing.Public = &CapabilitiesFilesSharingPublic{}
	}

	// c.Capabilities.Capabilities.FilesSharing.Public.Enabled is boolean

	if c.Capabilities.Capabilities.FilesSharing.Public.Password == nil {
		c.Capabilities.Capabilities.FilesSharing.Public.Password = &CapabilitiesFilesSharingPublicPassword{}
	}

	if c.Capabilities.Capabilities.FilesSharing.Public.Password.EnforcedFor == nil {
		c.Capabilities.Capabilities.FilesSharing.Public.Password.EnforcedFor = &CapabilitiesFilesSharingPublicPasswordEnforcedFor{}
	}

	// c.Capabilities.Capabilities.FilesSharing.Public.Password.EnforcedFor.ReadOnly is boolean
	// c.Capabilities.Capabilities.FilesSharing.Public.Password.EnforcedFor.ReadWrite is boolean
	// c.Capabilities.Capabilities.FilesSharing.Public.Password.EnforcedFor.UploadOnly is boolean

	// c.Capabilities.Capabilities.FilesSharing.Public.Password.Enforced is boolean

	if c.Capabilities.Capabilities.FilesSharing.Public.ExpireDate == nil {
		c.Capabilities.Capabilities.FilesSharing.Public.ExpireDate = &CapabilitiesFilesSharingPublicExpireDate{}
	}
	// c.Capabilities.Capabilities.FilesSharing.Public.ExpireDate.Enabled is boolean

	// c.Capabilities.Capabilities.FilesSharing.Public.SendMail is boolean
	// c.Capabilities.Capabilities.FilesSharing.Public.SocialShare is boolean
	// c.Capabilities.Capabilities.FilesSharing.Public.Upload is boolean
	// c.Capabilities.Capabilities.FilesSharing.Public.Multiple is boolean
	// c.Capabilities.Capabilities.FilesSharing.Public.SupportsUploadOnly is boolean

	if c.Capabilities.Capabilities.FilesSharing.User == nil {
		c.Capabilities.Capabilities.FilesSharing.User = &CapabilitiesFilesSharingUser{}
	}

	// c.Capabilities.Capabilities.FilesSharing.User.SendMail is boolean

	// c.Capabilities.Capabilities.FilesSharing.Resharing is boolean
	// c.Capabilities.Capabilities.FilesSharing.GroupSharing is boolean
	// c.Capabilities.Capabilities.FilesSharing.AutoAcceptShare is boolean
	// c.Capabilities.Capabilities.FilesSharing.ShareWithGroupMembersOnly is boolean
	// c.Capabilities.Capabilities.FilesSharing.ShareWithMembershipGroupsOnly is boolean

	if c.Capabilities.Capabilities.FilesSharing.UserEnumeration == nil {
		c.Capabilities.Capabilities.FilesSharing.UserEnumeration = &CapabilitiesFilesSharingUserEnumeration{}
	}

	// c.Capabilities.Capabilities.FilesSharing.UserEnumeration.Enabled is boolean
	// c.Capabilities.Capabilities.FilesSharing.UserEnumeration.GroupMembersOnly is boolean

	if c.Capabilities.Capabilities.FilesSharing.DefaultPermissions == 0 {
		c.Capabilities.Capabilities.FilesSharing.DefaultPermissions = 31
	}
	if c.Capabilities.Capabilities.FilesSharing.Federation == nil {
		c.Capabilities.Capabilities.FilesSharing.Federation = &CapabilitiesFilesSharingFederation{}
	}

	// c.Capabilities.Capabilities.FilesSharing.Federation.Outgoing is boolean
	// c.Capabilities.Capabilities.FilesSharing.Federation.Incoming is boolean

	if c.Capabilities.Capabilities.FilesSharing.SearchMinLength == 0 {
		c.Capabilities.Capabilities.FilesSharing.SearchMinLength = 2
	}

	// notifications

	if c.Capabilities.Capabilities.Notifications == nil {
		c.Capabilities.Capabilities.Notifications = &CapabilitiesNotifications{}
	}
	if c.Capabilities.Capabilities.Notifications.Endpoints == nil {
		c.Capabilities.Capabilities.Notifications.Endpoints = []string{"list", "get", "delete"}
	}

	// version

	if c.Capabilities.Version == nil {
		c.Capabilities.Version = &Version{
			// TODO get from build env
			Major:   10,
			Minor:   0,
			Micro:   9,
			String:  "10.0.9",
			Edition: "community",
		}
	}

}

type svc struct {
	c       *config
	handler http.Handler
}

// New returns a new capabilitiessvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	conf.init()

	s := &svc{
		c: conf,
	}
	s.setHandler()
	return s, nil
}

func (s *svc) Prefix() string {
	return s.c.Prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}
func (s *svc) Close() error {
	return nil
}
func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())

		head, tail := httpsvcs.ShiftPath(r.URL.Path)

		log.Debug().Str("head", head).Str("tail", tail).Msg("ocs routing")
		if head == "v1.php" {
			r.URL.Path = tail // write new tail back to response
			head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
			if head == "cloud" {
				head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
				if head == "user" {
					s.doUser(w, r)
					return
				} else if head == "capabilities" {
					s.doCapabilities(w, r)
					return
				}
			} else if head == "config" {
				s.doConfig(w, r)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	})
}
