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
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

type layoutTemplate struct {
	Username            string //the username
	UsernameLower       string //the username in lowercase
	UsernamePrefixCount string //first letters of username in lowercase eg: {{.UsernamePrefixCount.3}} will take the first 3 chars and make them lowercase, defaults to 1
	UsernameFirstLetter string //first letter of username in lowercase, equivalent as {{.UsernamePrefixCount.1}} but easy to read
	Provider            string //Provider/domain of user in lowercase
}

// GetUserHomePath converts username into user's home path according to layout
func GetUserHomePath(u *userpb.User, layout string) (string, error) {
	if u.Username == "" {
		return "", errors.Wrap(errtypes.UserRequired("userrequired"), "user has no username")
	}

	usernameSplit := strings.Split(u.Username, "@")
	if len(usernameSplit) == 1 {
		usernameSplit = append(usernameSplit, "_unknown")
	}
	if usernameSplit[1] == "" {
		usernameSplit[1] = "_unknown"
	}

	// handle {{.UsernamePrefixCount.x}}
	// where x is an int, pull it out and remove it from the go template
	letters := 1
	reg := regexp.MustCompile(`\{\{\.UsernamePrefixCount\.[0-9]+\}\}`)
	rmatches := reg.FindAllString(layout, -1)
	if rmatches != nil {
		reg := regexp.MustCompile("[^0-9]+")
		f, _ := strconv.ParseInt(reg.ReplaceAllString(rmatches[0], ""), 10, 64)
		if f > 1 {
			letters = int(f)
		}
		layout = strings.Replace(layout, "{{.UsernamePrefixCount."+strconv.Itoa(letters)+"}}", "{{.UsernamePrefixCount}}", -1)
	}

	pathTemplate := layoutTemplate{
		Username:            u.Username,
		UsernameLower:       strings.ToLower(u.Username),
		UsernamePrefixCount: strings.ToLower(string([]rune(usernameSplit[0])[0:letters])),
		UsernameFirstLetter: strings.ToLower(string([]rune(usernameSplit[0])[0])),
		Provider:            strings.ToLower(usernameSplit[1]),
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
