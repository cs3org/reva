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

package cs3api

import (
	"net/url"
	"regexp"

	"github.com/rs/zerolog"
)

func normalizeHost(domain string, log *zerolog.Logger) string {
	domainAddress := domain

	// See if the domain includes a protocol; if not, add one for easy parsing
	re := regexp.MustCompile(".*https?://.+")
	if !re.MatchString(domainAddress) {
		domainAddress = "https://" + domainAddress
	}

	// Parse the domain as a URL, ignoring errors
	domainURL, err := url.Parse(domainAddress)
	if err != nil {
		log.Error().Msgf("unable to parse host %v", domain)
		return domain
	}
	return domainURL.Hostname()
}

func normalizeURLPath(path string, log *zerolog.Logger) string {
	pathAddress := path

	// See if the path includes a protocol; if not, add a default one
	re := regexp.MustCompile(".*https?://.+")
	if !re.MatchString(pathAddress) {
		pathAddress = "https://" + pathAddress
	}

	// Try to parse the path
	pathURL, err := url.Parse(pathAddress)
	if err != nil {
		log.Error().Msgf("unable to parse URL %v", path)
		return path
	}
	return pathURL.String()
}
