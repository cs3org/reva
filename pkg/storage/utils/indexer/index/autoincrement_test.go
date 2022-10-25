// Copyright 2018-2022 CERN
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

package index_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cs3org/reva/v2/pkg/storage/utils/indexer/index"
	"github.com/cs3org/reva/v2/pkg/storage/utils/indexer/option"
	metadata "github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	"github.com/stretchr/testify/assert"
)

func TestNext(t *testing.T) {
	scenarios := []struct {
		name     string
		expected int
		indexBy  option.IndexBy
	}{
		{
			name:     "get next value",
			expected: 0,
			indexBy:  option.IndexByField("Number"),
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			tmpDir, err := createTmpDirStr()
			assert.NoError(t, err)
			dataDir := filepath.Join(tmpDir, "data")

			err = os.MkdirAll(dataDir, 0777)
			assert.NoError(t, err)

			storage, err := metadata.NewDiskStorage(dataDir)
			assert.NoError(t, err)

			i := index.NewAutoincrementIndex(
				storage,
				option.WithBounds(&option.Bound{
					Lower: 0,
					Upper: 0,
				}),
				option.WithFilesDir(dataDir),
				option.WithTypeName("LambdaType"),
				option.WithIndexBy(scenario.indexBy),
			)

			err = i.Init()
			assert.NoError(t, err)

			tmpFile, err := os.Create(filepath.Join(tmpDir, "data", "test-example"))
			assert.NoError(t, err)
			assert.NoError(t, tmpFile.Close())

			oldName, err := i.Add("test-example", "")
			assert.NoError(t, err)
			assert.Equal(t, "0", filepath.Base(oldName))

			oldName, err = i.Add("test-example", "")
			assert.NoError(t, err)
			assert.Equal(t, "1", filepath.Base(oldName))

			oldName, err = i.Add("test-example", "")
			assert.NoError(t, err)
			assert.Equal(t, "2", filepath.Base(oldName))
			t.Log(oldName)

			_ = os.RemoveAll(tmpDir)
		})
	}
}

func TestLowerBound(t *testing.T) {
	scenarios := []struct {
		name     string
		expected int
		indexBy  option.IndexBy
		entity   interface{}
	}{
		{
			name:     "get next value with a lower bound specified",
			expected: 0,
			indexBy:  option.IndexByField("Number"),
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			tmpDir, err := createTmpDirStr()
			assert.NoError(t, err)
			dataDir := filepath.Join(tmpDir, "data")

			err = os.MkdirAll(dataDir, 0777)
			assert.NoError(t, err)

			storage, err := metadata.NewDiskStorage(dataDir)
			assert.NoError(t, err)

			i := index.NewAutoincrementIndex(
				storage,
				option.WithBounds(&option.Bound{
					Lower: 1000,
				}),
				option.WithFilesDir(dataDir),
				option.WithTypeName("LambdaType"),
				option.WithIndexBy(scenario.indexBy),
			)

			err = i.Init()
			assert.NoError(t, err)

			tmpFile, err := os.Create(filepath.Join(tmpDir, "data", "test-example"))
			assert.NoError(t, err)
			assert.NoError(t, tmpFile.Close())

			oldName, err := i.Add("test-example", "")
			assert.NoError(t, err)
			assert.Equal(t, "1000", filepath.Base(oldName))

			oldName, err = i.Add("test-example", "")
			assert.NoError(t, err)
			assert.Equal(t, "1001", filepath.Base(oldName))

			oldName, err = i.Add("test-example", "")
			assert.NoError(t, err)
			assert.Equal(t, "1002", filepath.Base(oldName))
			t.Log(oldName)

			_ = os.RemoveAll(tmpDir)
		})
	}
}

func TestAdd(t *testing.T) {
	tmpDir, err := createTmpDirStr()
	assert.NoError(t, err)
	dataDir := filepath.Join(tmpDir, "data")

	err = os.MkdirAll(dataDir, 0777)
	assert.NoError(t, err)

	storage, err := metadata.NewDiskStorage(dataDir)
	assert.NoError(t, err)

	tmpFile, err := os.Create(filepath.Join(tmpDir, "data", "test-example"))
	assert.NoError(t, err)
	assert.NoError(t, tmpFile.Close())

	i := index.NewAutoincrementIndex(
		storage,
		option.WithBounds(&option.Bound{
			Lower: 0,
			Upper: 0,
		}),
		option.WithFilesDir(filepath.Join(tmpDir, "data")),
		option.WithTypeName("owncloud.Account"),
		option.WithIndexBy(option.IndexByField("UidNumber")),
	)

	err = i.Init()
	assert.NoError(t, err)

	_, err = i.Add("test-example", "")
	if err != nil {
		t.Error(err)
	}
}

func BenchmarkAdd(b *testing.B) {
	tmpDir, err := createTmpDirStr()
	assert.NoError(b, err)
	dataDir := filepath.Join(tmpDir, "data")

	err = os.MkdirAll(dataDir, 0777)
	assert.NoError(b, err)

	storage, err := metadata.NewDiskStorage(dataDir)
	assert.NoError(b, err)

	tmpFile, err := os.Create(filepath.Join(tmpDir, "data", "test-example"))
	assert.NoError(b, err)
	assert.NoError(b, tmpFile.Close())

	i := index.NewAutoincrementIndex(
		storage,
		option.WithBounds(&option.Bound{
			Lower: 0,
			Upper: 0,
		}),
		option.WithFilesDir(filepath.Join(tmpDir, "data")),
		option.WithTypeName("LambdaType"),
		option.WithIndexBy(option.IndexByField("Number")),
	)

	err = i.Init()
	assert.NoError(b, err)

	for n := 0; n < b.N; n++ {
		_, err := i.Add("test-example", "")
		if err != nil {
			b.Error(err)
		}
		assert.NoError(b, err)
	}
}

func createTmpDirStr() (string, error) {
	name, err := os.MkdirTemp("/tmp", "testfiles-*")
	if err != nil {
		return "", err
	}

	return name, nil
}
