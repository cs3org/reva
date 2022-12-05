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

package filelocks

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAcquireReadLock_Errors(t *testing.T) {
	l1, err := acquireLock("", false)
	assert.Nil(t, l1)
	assert.Equal(t, err, ErrPathEmpty)

	file, fin, _ := FileFactory()
	defer fin()

	l2, err := acquireLock(file, false)
	assert.NotNil(t, l2)
	assert.Nil(t, err)

	l3, err := acquireLock(file, false)
	assert.Nil(t, l3)
	assert.Equal(t, err, ErrAcquireLockFailed)
}

// utils

func FileFactory() (string, func(), error) {
	fu := func() {}
	tmpFile, err := os.CreateTemp(os.TempDir(), "flock")
	if err != nil {
		return "", fu, err
	}

	fu = func() {
		_ = os.Remove(tmpFile.Name())
	}

	err = tmpFile.Close()
	if err != nil {
		return "", fu, err
	}

	return tmpFile.Name(), fu, err
}
