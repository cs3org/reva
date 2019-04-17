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
	"net/http"
)

func (s *svc) doCapabilities(w http.ResponseWriter, r *http.Request) {
	res := &ocsResponse{
		OCS: &ocsPayload{
			Meta: ocsMetaOK,
			Data: &ocsCapabilitiesData{
				Capabilities: &ocsCapabilities{
					Core: &ocsCapabilitiesCore{
						PollInterval: 60, // TODO make configurable
						WebdavRoot:   "remote.php/webdav",
						Status: &ocsStatus{
							Installed:      true,
							Maintenance:    false,
							NeedsDBUpgrade: false,
							Version:        "10.0.9.5",  // TODO make build determined
							VersionString:  "10.0.9",    // TODO make build determined
							Edition:        "community", // TODO make build determined
							ProductName:    "ownCloud",  // TODO make configurable
							Hostname:       r.URL.Host,  // FIXME r.URL.Host is empty
						},
					},
					Checksums: &ocsCapabilitiesChecksums{
						SupportedTypes:      []string{"SHA1"},
						PreferredUploadType: "SHA1",
					},
					Files: &ocsCapabilitiesFiles{
						PrivateLinks:     true,
						BigFileChunking:  true,       // TODO is this old or new chunking? jfd: I guess old
						BlacklistedFiles: []string{}, // TODO make configurable
						Undelete:         true,
						Versioning:       true,
					},
					Dav: &ocsCapabilitiesDav{
						Chunking: "1.0", // TODO is this old or new chunking? jfd: I guess new
					},
					FilesSharing: &ocsCapabilitiesFilesSharing{
						APIEnabled: false, // TODO implement and make configurable
						Public: &ocsCapabilitiesFilesSharingPublic{
							Enabled: false, // TODO implement and make configurable
							Password: &ocsCapabilitiesFilesSharingPublicPassword{
								EnforcedFor: &ocsCapabilitiesFilesSharingPublicPasswordEnforcedFor{
									ReadOnly:   false, // TODO implement and make configurable
									ReadWrite:  false, // TODO implement and make configurable
									UploadOnly: false, // TODO implement and make configurable
								},
								Enforced: false, // TODO implement and make configurable
							},
							ExpireDate: &ocsCapabilitiesFilesSharingPublicExpireDate{
								Enabled: false,
							},
							SendMail:           false, // TODO implement and make configurable
							SocialShare:        false, // TODO implement and make configurable
							Upload:             false, // TODO implement and make configurable
							Multiple:           false, // TODO implement and make configurable
							SupportsUploadOnly: false, // TODO implement and make configurable
						},
						User: &ocsCapabilitiesFilesSharingUser{
							SendMail: false,
						},
						Resharing:                     false, // TODO implement and make configurable
						GroupSharing:                  false, // TODO implement and make configurable
						AutoAcceptShare:               false, // TODO implement and make configurable
						ShareWithGroupMembersOnly:     false, // TODO implement and make configurable
						ShareWithMembershipGroupsOnly: false, // TODO implement and make configurable
						UserEnumeration: &ocsCapabilitiesFilesSharingUserEnumeration{
							Enabled:          false,
							GroupMembersOnly: false,
						},
						DefaultPermissions: 31, // TODO use constant and make configurable
						Federation: &ocsCapabilitiesFilesSharingFederation{
							Outgoing: false,
							Incoming: false,
						},
						SearchMinLength: 2, // TODO implement and make configurable
					},
					Notifications: &ocsCapabilitiesNotifications{
						OCSEndpoints: []string{"list", "get", "delete"}, // TODO ?
					},
				},
				Version: &ocsVersion{
					Major:   10,
					Minor:   0,
					Micro:   9,
					String:  "10.0.9",
					Edition: "community",
				},
			},
		},
	}
	writeOCSResponse(w, r, res)
}
