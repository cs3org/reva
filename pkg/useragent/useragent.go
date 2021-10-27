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

package useragent

import (
	"strings"

	ua "github.com/mileusna/useragent"
)

func IsWeb(ua *ua.UserAgent) bool {
	return ua.IsChrome() || ua.IsEdge() || ua.IsFirefox() || ua.IsSafari() ||
		ua.IsInternetExplorer() || ua.IsOpera() || ua.IsOperaMini()
}

func IsMobile(ua *ua.UserAgent) bool {
	// workaround as the library does not recognise iOS string inside the user agent
	isIOS := ua.IsIOS() || strings.Contains(ua.String, "iOS")
	return !IsWeb(ua) && (ua.IsAndroid() || isIOS)
}

func IsDesktop(ua *ua.UserAgent) bool {
	return ua.Desktop && !IsWeb(ua)
}

func IsGRPC(ua *ua.UserAgent) bool {
	return strings.HasPrefix(ua.Name, "grpc")
}
