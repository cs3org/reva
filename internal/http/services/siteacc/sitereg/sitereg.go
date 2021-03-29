// Copyright 2018-2020 CERN
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

package sitereg

import (
	"fmt"
	"net/url"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/mentix/key"
	"github.com/cs3org/reva/pkg/mentix/utils/network"
)

// UnregisterSite unregister a site using the given site registration endpoint.
func UnregisterSite(serviceUrl string, apiKey key.APIKey, siteId key.SiteIdentifier, salt string) error {
	if len(serviceUrl) == 0 {
		return errors.Errorf("no site registration service URL provided")
	}

	if err := key.VerifyAPIKey(apiKey, salt); err != nil {
		return err
	}

	fullURL, err := url.Parse(serviceUrl)
	if err != nil {
		return errors.Wrap(err, "unable to parse the site registration service URL")
	}
	fullURL.Query().Add("action", "unregister")
	fullURL.Query().Add("apiKey", apiKey)
	fullURL.Query().Add("siteId", siteId)

	fmt.Println(fullURL.String())

	_, err = network.WriteEndpoint(fullURL, nil, true)
	if err != nil {
		return errors.Wrap(err, "unable to query the service registration endpoint")
	}

	return nil
}
