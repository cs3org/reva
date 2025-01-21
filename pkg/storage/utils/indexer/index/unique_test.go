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
	"path"
	"testing"

	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/indexer/errors"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/indexer/index"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/indexer/option"
	. "github.com/opencloud-eu/reva/v2/pkg/storage/utils/indexer/test"
	"github.com/opencloud-eu/reva/v2/pkg/storage/utils/metadata"
	"github.com/stretchr/testify/assert"
)

func TestUniqueLookupSingleEntry(t *testing.T) {
	uniq, dataDir := getUniqueIdxSut(t, option.IndexByField("Email"), User{})
	filesDir := path.Join(dataDir, "users")

	t.Log("existing lookup")
	resultPath, err := uniq.Lookup("mikey@example.com")
	assert.NoError(t, err)

	assert.Equal(t, []string{path.Join(filesDir, "abcdefg-123")}, resultPath)

	t.Log("non-existing lookup")
	resultPath, err = uniq.Lookup("doesnotExists@example.com")
	assert.Error(t, err)
	assert.IsType(t, &errors.NotFoundErr{}, err)
	assert.Empty(t, resultPath)

	_ = os.RemoveAll(dataDir)

}

func TestUniqueUniqueConstraint(t *testing.T) {
	uniq, dataDir := getUniqueIdxSut(t, option.IndexByField("Email"), User{})

	_, err := uniq.Add("abcdefg-123", "mikey@example.com")
	assert.Error(t, err)
	assert.IsType(t, &errors.AlreadyExistsErr{}, err)

	_ = os.RemoveAll(dataDir)
}

func TestUniqueRemove(t *testing.T) {
	uniq, dataDir := getUniqueIdxSut(t, option.IndexByField("Email"), User{})

	err := uniq.Remove("", "mikey@example.com")
	assert.NoError(t, err)

	_, err = uniq.Lookup("mikey@example.com")
	assert.Error(t, err)
	assert.IsType(t, &errors.NotFoundErr{}, err)

	_ = os.RemoveAll(dataDir)
}

func TestUniqueUpdate(t *testing.T) {
	uniq, dataDir := getUniqueIdxSut(t, option.IndexByField("Email"), User{})

	t.Log("successful update")
	err := uniq.Update("", "mikey@example.com", "mikey2@example.com")
	assert.NoError(t, err)

	t.Log("failed update because not found")
	err = uniq.Update("", "nonexisting@example.com", "something2@example.com")
	assert.Error(t, err)
	assert.IsType(t, &errors.NotFoundErr{}, err)

	_ = os.RemoveAll(dataDir)
}

func TestUniqueIndexSearch(t *testing.T) {
	sut, dataDir := getUniqueIdxSut(t, option.IndexByField("Email"), User{})

	res, err := sut.Search("j*@example.com")

	assert.NoError(t, err)
	assert.Len(t, res, 2)

	assert.Equal(t, "ewf4ofk-555", path.Base(res[0]))
	assert.Equal(t, "rulan54-777", path.Base(res[1]))

	_, err = sut.Search("does-not-exist@example.com")
	assert.Error(t, err)
	assert.IsType(t, &errors.NotFoundErr{}, err)

	_ = os.RemoveAll(dataDir)
}

func getUniqueIdxSut(t *testing.T, indexBy option.IndexBy, entityType interface{}) (index.Index, string) {
	dataPath, _ := WriteIndexTestData(Data, "ID", "")
	storage, err := metadata.NewDiskStorage(dataPath)
	if err != nil {
		t.Fatal(err)
	}

	sut := index.NewUniqueIndexWithOptions(
		storage,
		option.WithTypeName(GetTypeFQN(entityType)),
		option.WithIndexBy(indexBy),
		option.WithFilesDir(path.Join(dataPath, "users")),
	)
	err = sut.Init()
	if err != nil {
		t.Fatal(err)
	}

	for _, u := range Data["users"] {
		pkVal := ValueOf(u, "ID")
		idxByVal := ValueOf(u, "Email")
		_, err := sut.Add(pkVal, idxByVal)
		if err != nil {
			t.Fatal(err)
		}
	}

	return sut, dataPath
}
