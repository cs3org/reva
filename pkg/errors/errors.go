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

package errors

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

func packageName() string {
	pc, _, _, _ := runtime.Caller(2)
	parts := strings.Split(runtime.FuncForPC(pc).Name(), ".")
	pl := len(parts)
	pkg := ""
	if parts[pl-2][0] == '(' {
		pkg = strings.Join(parts[0:pl-2], ".")
	} else {
		pkg = strings.Join(parts[0:pl-1], ".")
	}
	return path.Base(pkg)
}

// Newf is a wrapper on top of errors.New which prefixes the error with the name
// of the package from which it was called.
func Newf(format string, args ...interface{}) error {
	return errors.Wrap(errors.New(fmt.Sprintf(format, args...)), packageName())
}

// Wrapf is a wrapper on top of errors.Wrapf which prefixes the wrapped error
// with the name of the package from which it was called.
func Wrapf(err error, format string, args ...interface{}) error {
	return errors.Wrap(errors.Wrapf(err, format, args...), packageName())
}
