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

package helper

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

type layoutTemplate struct {
	Username    string
	FirstLetter string
	Provider    string
}

func GetUserHomePath(u *userpb.User, layout string) (string, error) {
	if u.Username == "" {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), "user has no username")
	}

	usernameSplit := strings.Split(u.Username, "@")
	if len(usernameSplit) == 1 {
		usernameSplit = append(usernameSplit, "_Unknown")
	}
	if usernameSplit[1] == "" {
		usernameSplit[1] = "_Unknown"
	}

	pathTemplate := layoutTemplate{
		Username:    u.Username,
		FirstLetter: strings.ToLower(string([]rune(usernameSplit[0])[0])),
		Provider:    usernameSplit[1],
	}
	tmpl, err := template.New("userhomepath").Parse(layout)
	if err != nil {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), fmt.Sprintf("template parse error: %s", err.Error()))
	}
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, pathTemplate)
	if err != nil {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), fmt.Sprintf("template execute error: %s", err.Error()))
	}

	return buf.String(), nil
}
