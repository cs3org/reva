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

package share

import (
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// Defaults for webapp shares created by this server, according to
// the OCM specifications: the webapp protocol requires non-empty
// requirements (including `must-exchange-token`) and targets.
var (
	DefaultWebappRequirements = []string{"must-exchange-token"}
	DefaultWebappTargets      = []string{"blank"}
)

// GetRole derives the auth role (and its string form) granted by an OCM share.
// It assumes all access methods carry consistent permissions and inspects the first match.
func GetRole(s *ocm.Share) (authpb.Role, string) {
	for _, m := range s.AccessMethods {
		switch v := m.Term.(type) {
		case *ocm.AccessMethod_WebdavOptions:
			p := v.WebdavOptions.Permissions
			if p.InitiateFileUpload {
				return authpb.Role_ROLE_EDITOR, "editor"
			}
			if p.InitiateFileDownload {
				return authpb.Role_ROLE_VIEWER, "viewer"
			}
		case *ocm.AccessMethod_WebappOptions:
			p := v.WebappOptions.Permissions
			if p.InitiateFileUpload {
				return authpb.Role_ROLE_EDITOR, "editor"
			}
			if p.Stat {
				return authpb.Role_ROLE_VIEWER, "viewer"
			}
		}
	}
	return authpb.Role_ROLE_INVALID, "invalid"
}

// NewWebDAVProtocol is an abstraction for creating a WebDAV protocol.
func NewWebDAVProtocol(uri, sharedSecret string, perms *ocm.SharePermissions, accTypes []ocm.AccessType, reqs []string) *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_WebdavOptions{
			WebdavOptions: &ocm.WebDAVProtocol{
				Uri:          uri,
				SharedSecret: sharedSecret,
				Permissions:  perms,
				AccessTypes:  accTypes,
				Requirements: reqs,
			},
		},
	}
}

// NewWebappProtocol is an abstraction for creating a Webapp protocol.
func NewWebappProtocol(uri, sharedSecret string, perms *provider.ResourcePermissions, reqs, targets []string, appName, appIconHint string, mediaTypes []string) *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_WebappOptions{
			WebappOptions: &ocm.WebappProtocol{
				Uri:              uri,
				SharedSecret:     sharedSecret,
				SharePermissions: perms,
				Requirements:     reqs,
				Targets:          targets,
				AppName:          appName,
				AppIconHint:      appIconHint,
				MediaTypes:       mediaTypes,
			},
		},
	}
}

// NewEmbeddedProtocol is an abstraction for creating an OCM embedded protocol.
func NewEmbeddedProtocol(payload string) *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_EmbeddedOptions{
			EmbeddedOptions: &ocm.EmbeddedProtocol{
				Payload: payload,
			},
		},
	}
}

// NewWebDavAccessMethod is an abstraction for creating a WebDAV access method,
// which is the protocol used by remote users to access an OCM share.
func NewWebDavAccessMethod(perms *provider.ResourcePermissions, accTypes []ocm.AccessType, reqs []string) *ocm.AccessMethod {
	return &ocm.AccessMethod{
		Term: &ocm.AccessMethod_WebdavOptions{
			WebdavOptions: &ocm.WebDAVAccessMethod{
				Permissions:  perms,
				AccessTypes:  accTypes,
				Requirements: reqs,
			},
		},
	}
}

// NewWebappAccessMethod is an abstraction for creating a Webapp access method,
// which is the protocol used by remote users to access an OCM share.
func NewWebappAccessMethod(perms *provider.ResourcePermissions, reqs []string, appName string) *ocm.AccessMethod {
	return &ocm.AccessMethod{
		Term: &ocm.AccessMethod_WebappOptions{
			WebappOptions: &ocm.WebappAccessMethod{
				Permissions:  perms,
				Requirements: reqs,
				AppName:      appName,
			},
		},
	}
}
