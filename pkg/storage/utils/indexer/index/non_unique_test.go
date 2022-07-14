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
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/cs3org/reva/v2/pkg/storage/utils/indexer/errors"
	"github.com/cs3org/reva/v2/pkg/storage/utils/indexer/index"
	"github.com/cs3org/reva/v2/pkg/storage/utils/indexer/option"
	. "github.com/cs3org/reva/v2/pkg/storage/utils/indexer/test"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	"github.com/stretchr/testify/assert"
)

func TestNonUniqueIndexAdd(t *testing.T) {
	sut, dataPath := getNonUniqueIdxSut(t, Pet{}, option.IndexByField("Color"))

	ids, err := sut.Lookup("Green")
	assert.NoError(t, err)
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, "goefe-789")
	assert.Contains(t, ids, "xadaf-189")

	ids, err = sut.Lookup("White")
	assert.NoError(t, err)
	assert.EqualValues(t, []string{"wefwe-456"}, ids)

	ids, err = sut.Lookup("Cyan")
	assert.Error(t, err)
	assert.Nil(t, ids)

	_ = os.RemoveAll(dataPath)

}

func TestNonUniqueIndexUpdate(t *testing.T) {
	sut, dataPath := getNonUniqueIdxSut(t, Pet{}, option.IndexByField("Color"))

	err := sut.Update("goefe-789", "Green", "Black")
	assert.NoError(t, err)

	err = sut.Update("xadaf-189", "Green", "Black")
	assert.NoError(t, err)

	assert.DirExists(t, path.Join(dataPath, fmt.Sprintf("index.disk/non_unique.%v.Color/Black", GetTypeFQN(Pet{}))))
	assert.NoDirExists(t, path.Join(dataPath, fmt.Sprintf("index.disk/non_unique.%v.Color/Green", GetTypeFQN(Pet{}))))

	_ = os.RemoveAll(dataPath)
}

func TestNonUniqueIndexDelete(t *testing.T) {
	sut, dataPath := getNonUniqueIdxSut(t, Pet{}, option.IndexByField("Color"))
	assert.FileExists(t, path.Join(dataPath, fmt.Sprintf("index.disk/non_unique.%v.Color/Green/goefe-789", GetTypeFQN(Pet{}))))

	err := sut.Remove("goefe-789", "Green")
	assert.NoError(t, err)
	assert.NoFileExists(t, path.Join(dataPath, fmt.Sprintf("index.disk/non_unique.%v.Color/Green/goefe-789", GetTypeFQN(Pet{}))))
	assert.FileExists(t, path.Join(dataPath, fmt.Sprintf("index.disk/non_unique.%v.Color/Green/xadaf-189", GetTypeFQN(Pet{}))))

	_ = os.RemoveAll(dataPath)
}

func TestNonUniqueIndexSearch(t *testing.T) {
	sut, dataPath := getNonUniqueIdxSut(t, Pet{}, option.IndexByField("Email"))

	res, err := sut.Search("Gr*")

	assert.NoError(t, err)
	assert.Len(t, res, 2)

	assert.Equal(t, "goefe-789", path.Base(res[0]))
	assert.Equal(t, "xadaf-189", path.Base(res[1]))

	_, err = sut.Search("does-not-exist@example.com")
	assert.Error(t, err)
	assert.IsType(t, &errors.NotFoundErr{}, err)

	_ = os.RemoveAll(dataPath)
}

// entity: used to get the fully qualified name for the index root path.
func getNonUniqueIdxSut(t *testing.T, entity interface{}, indexBy option.IndexBy) (index.Index, string) {
	dataPath, _ := WriteIndexTestData(Data, "ID", "")
	storage, err := metadata.NewDiskStorage(dataPath)
	if err != nil {
		t.Fatal(err)
	}

	sut := index.NewNonUniqueIndexWithOptions(
		storage,
		option.WithTypeName(GetTypeFQN(entity)),
		option.WithIndexBy(indexBy),
		option.WithFilesDir(path.Join(dataPath, "pets")),
	)
	err = sut.Init()
	if err != nil {
		t.Fatal(err)
	}

	for _, u := range Data["pets"] {
		pkVal := ValueOf(u, "ID")
		idxByVal := ValueOf(u, "Color")
		_, err := sut.Add(pkVal, idxByVal)
		if err != nil {
			t.Fatal(err)
		}
	}

	return sut, dataPath
}
