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

/*
Package templates contains data-driven templates for path layouts.

Templates can use functions from the gitbub.com/Masterminds/sprig library.
All templates are cleaned with path.Clean().
*/
package templates

import (
	"bytes"
	"fmt"
	"path"
	"text/template"

	"github.com/Masterminds/sprig"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/pkg/errors"
)

// UserData contains the template placeholders for a user.
// For example {{.Username}} or {{.Id.Idp}}
type UserData struct {
	*userpb.User
}

func WithUser(u *userpb.User, tpl string) string {
	tpl = clean(tpl)
	ut := newUserData(u)
	// compile given template tpl
	t, err := template.New("tpl").Funcs(sprig.TxtFuncMap()).Parse(tpl)
	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error parsing template: user_template:%+v tpl:%s", ut, tpl))
		panic(err)
	}
	b := bytes.Buffer{}
	if err := t.Execute(&b, u); err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error executing template: user_template:%+v tpl:%s", ut, tpl))
		panic(err)
	}
	return b.String()
}

func newUserData(u *userpb.User) *UserData {
	ut := &UserData{User: u}
	return ut
}

func clean(a string) string {
	return path.Clean(a)
}
