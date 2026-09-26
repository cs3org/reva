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

package ocmshareprovider

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"google.golang.org/protobuf/proto"
)

const (
	// invalidShareAccessMethodsText is the only gRPC message returned when
	// normalized access methods are rejected. It carries no request content.
	invalidShareAccessMethodsText = "invalid ocm share access methods"
	// webappOnlyNoProtocolText is returned when a webapp-only offer normalizes
	// to zero protocols. It carries no request content.
	webappOnlyNoProtocolText = "webapp-only offer has no shareable protocol"
	// webappOmissionReason is logged once when a requested webapp candidate is
	// left off the share. It has no URL, name, secret, or token.
	webappOmissionReason = "omitting requested webapp access method"
	reqMustExchangeToken = "must-exchange-token"
	reqMustUseMFA        = "must-use-mfa"
)

var errInvalidShareAccessMethods = errors.New(invalidShareAccessMethodsText)

// validateWebappOffer checks the provider-local offer at startup.
// Disabled mode accepts an omitted name and endpoint. Enabled mode keeps the
// configured name exactly, including padding, and requires an absolute http
// or https opener with a hostname and no userinfo.
func (c *config) validateWebappOffer() error {
	if c == nil || !c.OfferWebapp {
		return nil
	}
	if strings.TrimSpace(c.WebappName) == "" {
		return errtypes.BadRequest("ocmshareprovider: webapp_name must be non-empty when offer_webapp is true")
	}
	parsed, err := url.Parse(c.WebAppEndpoint)
	if err != nil || !validOutboundWebappEndpoint(parsed) {
		return errtypes.BadRequest("ocmshareprovider: webapp_endpoint must be an absolute URL with a hostname when offer_webapp is true")
	}
	return nil
}

// validOutboundWebappEndpoint reports whether parsed is an absolute http or
// https URL with a hostname and no userinfo.
func validOutboundWebappEndpoint(parsed *url.URL) bool {
	if parsed == nil || parsed.User != nil {
		return false
	}
	if !parsed.IsAbs() || parsed.Hostname() == "" {
		return false
	}
	switch parsed.Scheme {
	case "http", "https":
		return true
	default:
		return false
	}
}

// webappURL returns the configured opener unchanged. The share id is not
// appended and no /lab suffix is added.
func (s *service) webappURL() string {
	return s.conf.WebAppEndpoint
}

func (s *service) getWebappProtocol(ocmShare *ocm.Share, m *ocm.AccessMethod_WebappOptions) *ocmd.Webapp {
	opts := m.WebappOptions
	var perms []string
	if opts.GetPermissions().GetInitiateFileDownload() {
		perms = append(perms, "read")
	} else {
		perms = append(perms, "view")
	}
	if opts.GetPermissions().GetInitiateFileUpload() {
		perms = append(perms, "write")
	}
	if opts.GetPermissions().GetAddGrant() {
		perms = append(perms, "share")
	}

	return &ocmd.Webapp{
		URI:          s.webappURL(),
		SharedSecret: ocmShare.Token,
		Permissions:  perms,
		Requirements: cloneStrings(opts.GetRequirements()),
		Targets:      cloneDefaultWebappTargets(),
		AppName:      opts.GetAppName(),
	}
}

// freshAccessMethods clones the request methods that will be stored and sent.
// webappSupported and tokenExchangeSupported are computed by the caller;
// this helper does not query discovery. The offer gate is decided before any
// webapp candidate is validated, so a candidate that will be omitted cannot
// fail the share.
func (s *service) freshAccessMethods(
	ctx context.Context,
	requested []*ocm.AccessMethod,
	webappSupported bool,
	tokenExchangeSupported bool,
) ([]*ocm.AccessMethod, error) {
	if s == nil || s.conf == nil {
		return nil, errInvalidShareAccessMethods
	}

	hasCandidate, err := hasWebappCandidate(requested)
	if err != nil {
		return nil, err
	}
	attach := s.conf.OfferWebapp && hasCandidate && webappSupported && tokenExchangeSupported
	if hasCandidate && !attach {
		appctx.GetLogger(ctx).Info().Msg(webappOmissionReason)
	}
	if attach {
		count, err := countWebappCandidates(requested)
		if err != nil {
			return nil, err
		}
		if count >= 2 {
			return nil, errInvalidShareAccessMethods
		}
	}

	methods := make([]*ocm.AccessMethod, 0, len(requested))
	var webappReqs []string
	attached := false
	for _, m := range requested {
		if m == nil {
			continue
		}
		webapp, classErr := isWebappAccessMethod(m)
		if classErr != nil && !webapp {
			return nil, classErr
		}
		if webapp {
			if !attach {
				continue
			}
			if classErr != nil {
				return nil, classErr
			}
			normalized, err := s.normalizeAttachedWebapp(m)
			if err != nil {
				return nil, err
			}
			methods = append(methods, normalized)
			webappReqs = cloneStrings(normalized.GetWebappOptions().GetRequirements())
			attached = true
			continue
		}
		retained, err := cloneRetainedMethod(m)
		if err != nil {
			return nil, err
		}
		methods = append(methods, retained)
	}
	if attached {
		if err := alignRetainedWebDAV(methods, webappReqs); err != nil {
			return nil, err
		}
	}
	return methods, nil
}

func hasWebappCandidate(methods []*ocm.AccessMethod) (bool, error) {
	for _, m := range methods {
		webapp, err := isWebappAccessMethod(m)
		if err != nil && !webapp {
			return false, err
		}
		if webapp {
			return true, nil
		}
	}
	return false, nil
}

func countWebappCandidates(methods []*ocm.AccessMethod) (int, error) {
	count := 0
	for _, m := range methods {
		webapp, err := isWebappAccessMethod(m)
		if err != nil && !webapp {
			return 0, err
		}
		if webapp {
			count++
		}
	}
	return count, nil
}

// isWebappAccessMethod reports whether m is a webapp candidate.
// A typed-nil webapp term is still a candidate. Its error is ignored until
// the offer gate is true, so omitted candidates are not validated.
func isWebappAccessMethod(m *ocm.AccessMethod) (bool, error) {
	if m == nil || m.Term == nil {
		return false, nil
	}
	term, ok := m.Term.(*ocm.AccessMethod_WebappOptions)
	if !ok {
		return false, nil
	}
	if term == nil || term.WebappOptions == nil {
		return true, errInvalidShareAccessMethods
	}
	return true, nil
}

func cloneAccessMethod(m *ocm.AccessMethod) (cloned *ocm.AccessMethod, err error) {
	if m == nil {
		return nil, errInvalidShareAccessMethods
	}
	defer func() {
		if recover() != nil {
			cloned = nil
			err = errInvalidShareAccessMethods
		}
	}()

	clonedMsg := proto.Clone(m)
	if clonedMsg == nil {
		return nil, errInvalidShareAccessMethods
	}
	var ok bool
	cloned, ok = clonedMsg.(*ocm.AccessMethod)
	if !ok || cloned == nil {
		return nil, errInvalidShareAccessMethods
	}
	return cloned, nil
}

func cloneRetainedMethod(m *ocm.AccessMethod) (*ocm.AccessMethod, error) {
	if err := validateRetainedWebDAV(m); err != nil {
		return nil, err
	}
	return cloneAccessMethod(m)
}

func validateRetainedWebDAV(m *ocm.AccessMethod) error {
	if m == nil || m.Term == nil {
		return nil
	}
	term, ok := m.Term.(*ocm.AccessMethod_WebdavOptions)
	if !ok {
		return nil
	}
	if term == nil || term.WebdavOptions == nil || term.WebdavOptions.Permissions == nil {
		return errInvalidShareAccessMethods
	}
	return nil
}

func (s *service) normalizeAttachedWebapp(m *ocm.AccessMethod) (*ocm.AccessMethod, error) {
	cloned, err := cloneAccessMethod(m)
	if err != nil {
		return nil, err
	}
	opts := cloned.GetWebappOptions()
	if opts == nil {
		return nil, errInvalidShareAccessMethods
	}
	reqs, err := normalizeWebappRequirements(opts.GetRequirements())
	if err != nil {
		return nil, err
	}
	opts.AppName = s.conf.WebappName
	opts.Requirements = reqs
	return cloned, nil
}

func normalizeWebappRequirements(in []string) ([]string, error) {
	if len(in) == 0 {
		return cloneDefaultWebappRequirements(), nil
	}
	if !slices.Contains(in, reqMustExchangeToken) {
		return nil, errInvalidShareAccessMethods
	}
	return normalizeRequirementList(in, knownOutboundWebappRequirement)
}

func knownOutboundWebappRequirement(req string) bool {
	switch req {
	case reqMustExchangeToken, reqMustUseMFA:
		return true
	default:
		return false
	}
}

func alignRetainedWebDAV(methods []*ocm.AccessMethod, webappReqs []string) error {
	if slices.Contains(webappReqs, reqMustUseMFA) && hasRetainedWebDAV(methods) {
		return errInvalidShareAccessMethods
	}
	for _, m := range methods {
		term, ok := m.Term.(*ocm.AccessMethod_WebdavOptions)
		if !ok {
			continue
		}
		if term == nil || term.WebdavOptions == nil || term.WebdavOptions.Permissions == nil {
			return errInvalidShareAccessMethods
		}
		if len(term.WebdavOptions.Requirements) == 0 {
			term.WebdavOptions.Requirements = cloneDefaultWebappRequirements()
			continue
		}
		normalized, err := normalizeWebDAVRequirements(term.WebdavOptions.Requirements)
		if err != nil {
			return err
		}
		if !sameStringSet(normalized, webappReqs) {
			return errInvalidShareAccessMethods
		}
		term.WebdavOptions.Requirements = normalized
	}
	return nil
}

func hasRetainedWebDAV(methods []*ocm.AccessMethod) bool {
	for _, m := range methods {
		if m == nil || m.Term == nil {
			continue
		}
		if _, ok := m.Term.(*ocm.AccessMethod_WebdavOptions); ok {
			return true
		}
	}
	return false
}

func normalizeWebDAVRequirements(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, errInvalidShareAccessMethods
	}
	return normalizeRequirementList(in, func(req string) bool {
		return req == reqMustExchangeToken
	})
}

func normalizeRequirementList(in []string, known func(string) bool) ([]string, error) {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, req := range in {
		if req == "" || strings.TrimSpace(req) != req || !known(req) {
			return nil, errInvalidShareAccessMethods
		}
		if _, ok := seen[req]; ok {
			return nil, errInvalidShareAccessMethods
		}
		seen[req] = struct{}{}
		out = append(out, req)
	}
	return out, nil
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, value := range a {
		seen[value]++
	}
	for _, value := range b {
		seen[value]--
		if seen[value] < 0 {
			return false
		}
	}
	return true
}

func cloneDefaultWebappRequirements() []string {
	return cloneStrings(share.DefaultWebappRequirements)
}

func cloneDefaultWebappTargets() []string {
	return cloneStrings(share.DefaultWebappTargets)
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
