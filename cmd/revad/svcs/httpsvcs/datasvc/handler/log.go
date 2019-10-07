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

// this file is a fork of https://github.com/tus/tusd/tree/master/pkg/handler/log.go
// TODO remove when PRs have been merged upstream

package handler

import (
	"log"
)

func (h *UnroutedHandler) log(eventName string, details ...string) {
	LogEvent(h.logger, eventName, details...)
}

func LogEvent(logger *log.Logger, eventName string, details ...string) {
	result := make([]byte, 0, 100)

	result = append(result, `event="`...)
	result = append(result, eventName...)
	result = append(result, `" `...)

	for i := 0; i < len(details); i += 2 {
		result = append(result, details[i]...)
		result = append(result, `="`...)
		result = append(result, details[i+1]...)
		result = append(result, `" `...)
	}

	result = append(result, "\n"...)
	logger.Output(2, string(result))
}
