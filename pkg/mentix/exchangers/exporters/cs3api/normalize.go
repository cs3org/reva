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

package cs3api

import (
	"net/url"
	"regexp"

	"github.com/rs/zerolog"
)

func _normalizeAddress(addr string) (*url.URL, error) {
	address := addr

	// See if the address includes a protocol; if not, add a default one
	re := regexp.MustCompile(".+://.+")
	if !re.MatchString(address) {
		address = "https://" + address
	}

	// Parse the address as a URL
	addressURL, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	return addressURL, nil
}

func normalizeHost(domain string, log *zerolog.Logger) string {
	address, err := _normalizeAddress(domain)
	if err != nil {
		log.Error().Msgf("unable to parse host %v", domain)
		return domain
	}
	return address.Hostname()
}

func normalizeURLPath(path string, log *zerolog.Logger) string {
	address, err := _normalizeAddress(path)
	if err != nil {
		log.Error().Msgf("unable to parse URL %v", path)
		return path
	}
	return address.String()
}
