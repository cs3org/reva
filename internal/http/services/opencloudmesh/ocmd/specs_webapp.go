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

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// implementedWebappTarget is the only target this receiver can open.
const implementedWebappTarget = "blank"

// ErrInvalidProtocolURI is the stable sentinel for a malformed protocol URI.
var ErrInvalidProtocolURI = errors.New("invalid protocol uri")

// ErrWebappMFAUnproven means this receiver cannot satisfy must-use-mfa.
// The requirement is recognized and permanently rejected. No session proof
// is checked or claimed.
var ErrWebappMFAUnproven = errors.New("protocol webapp requirement must-use-mfa cannot be satisfied by this receiver")

// ValidateReceived checks a webapp offer against this receiver's targets.
// Empty AppName is valid metadata. The offer is not mutated.
func (w *Webapp) ValidateReceived(receiverTargets []string) error {
	if w == nil {
		return errors.New("nil webapp protocol")
	}
	if strings.TrimSpace(w.SharedSecret) == "" {
		return errors.New("protocol webapp missing sharedSecret")
	}
	if strings.TrimSpace(w.SharedSecret) != w.SharedSecret {
		return errors.New("protocol webapp missing sharedSecret")
	}
	if err := validateWebappRequirements(w.Requirements); err != nil {
		return err
	}
	if err := validateSharedProtocolFields(
		"webapp",
		w.SharedSecret,
		w.Permissions,
		validWebappPermissions,
		w.Requirements,
		validWebappRequirements,
		w.URI,
	); err != nil {
		return err
	}
	if _, err := ValidateAbsoluteWebappURI(w.URI); err != nil {
		return err
	}
	return webappTargetCompatible(w.Targets, receiverTargets)
}

// ValidateWebappLaunch rechecks a stored webapp before any remote call.
// Permissions are not re-derived. Empty AppName stays valid.
func ValidateWebappLaunch(uri, secret string, requirements, targets, receiverTargets []string) error {
	offer := &Webapp{
		URI:          uri,
		SharedSecret: secret,
		Permissions:  []string{"view"},
		Requirements: requirements,
		Targets:      targets,
	}
	return offer.ValidateReceived(receiverTargets)
}

func validateWebappRequirements(requirements []string) error {
	for _, requirement := range requirements {
		if strings.TrimSpace(requirement) == "" || strings.TrimSpace(requirement) != requirement {
			return errors.New("protocol webapp has malformed requirement")
		}
		if _, ok := validWebappRequirements[requirement]; !ok {
			return fmt.Errorf("protocol webapp has unsupported requirement %q", requirement)
		}
		if requirement == "must-use-mfa" {
			return ErrWebappMFAUnproven
		}
	}
	if !slices.Contains(requirements, "must-exchange-token") {
		return errors.New("protocol webapp requirements must include must-exchange-token")
	}
	return nil
}

func webappTargetCompatible(offered, advertised []string) error {
	if len(offered) == 0 {
		return errors.New("protocol webapp missing targets")
	}
	if err := validateVocabulary("webapp", "target", offered, validWebappTargets); err != nil {
		return err
	}
	offeredBlank := slices.Contains(offered, implementedWebappTarget)
	advertisedBlank := slices.Contains(advertised, implementedWebappTarget)
	if offeredBlank && advertisedBlank {
		return nil
	}
	return errors.New("protocol webapp has no compatible target")
}

// ValidateAbsoluteWebappURI returns raw when it is a non-blank, unpadded,
// absolute http or https URI with a hostname and no userinfo. Relative,
// network-path, and opaque references are rejected. Escaping, query, and
// fragment are preserved.
func ValidateAbsoluteWebappURI(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("protocol webapp missing uri")
	}
	if strings.TrimSpace(raw) != raw {
		return "", invalidProtocolURI("webapp")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", invalidProtocolURI("webapp")
	}
	if err := requireAbsoluteHTTPURL(parsed); err != nil {
		return "", invalidProtocolURI("webapp")
	}
	return raw, nil
}

func invalidProtocolURI(protocolName string) error {
	return fmt.Errorf("protocol %s has invalid uri: %w", protocolName, ErrInvalidProtocolURI)
}

func requireAbsoluteHTTPURL(parsed *url.URL) error {
	if parsed == nil || parsed.Opaque != "" || parsed.User != nil {
		return ErrInvalidProtocolURI
	}
	if !parsed.IsAbs() || parsed.Host == "" || parsed.Hostname() == "" {
		return ErrInvalidProtocolURI
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrInvalidProtocolURI
	}
	if parsed.Host == "http:" || parsed.Host == "https:" || strings.Contains(parsed.Host, "://") {
		return ErrInvalidProtocolURI
	}
	if strings.HasPrefix(parsed.Path, "//http://") || strings.HasPrefix(parsed.Path, "//https://") {
		return ErrInvalidProtocolURI
	}
	return nil
}
