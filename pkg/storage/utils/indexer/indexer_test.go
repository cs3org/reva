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

package indexer

import (
	"context"
	"os"
	"path"
	"testing"

	_ "github.com/owncloud/reva/v2/pkg/storage/utils/indexer/index"
	"github.com/owncloud/reva/v2/pkg/storage/utils/indexer/option"
	. "github.com/owncloud/reva/v2/pkg/storage/utils/indexer/test"
	"github.com/owncloud/reva/v2/pkg/storage/utils/metadata"
	"github.com/stretchr/testify/assert"
)

func TestIndexer_Disk_FindByWithUniqueIndex(t *testing.T) {
	dataDir, err := WriteIndexTestData(Data, "ID", "")
	assert.NoError(t, err)
	indexer := createDiskIndexer(dataDir)

	err = indexer.AddIndex(&User{}, option.IndexByField("UserName"), "ID", "users", "unique", nil, false)
	assert.NoError(t, err)

	u := &User{ID: "abcdefg-123", UserName: "mikey", Email: "mikey@example.com"}
	_, err = indexer.Add(u)
	assert.NoError(t, err)

	res, err := indexer.FindBy(User{}, NewField("UserName", "mikey"))
	assert.NoError(t, err)
	t.Log(res)

	_ = os.RemoveAll(dataDir)
}

func TestIndexer_Disk_AddWithUniqueIndex(t *testing.T) {
	dataDir, err := WriteIndexTestData(Data, "ID", "")
	assert.NoError(t, err)
	indexer := createDiskIndexer(dataDir)

	err = indexer.AddIndex(&User{}, option.IndexByField("UserName"), "ID", "users", "unique", nil, false)
	assert.NoError(t, err)

	u := &User{ID: "abcdefg-123", UserName: "mikey", Email: "mikey@example.com"}
	_, err = indexer.Add(u)
	assert.NoError(t, err)

	_ = os.RemoveAll(dataDir)
}

func TestIndexer_Disk_AddWithNonUniqueIndex(t *testing.T) {
	dataDir, err := WriteIndexTestData(Data, "ID", "")
	assert.NoError(t, err)
	indexer := createDiskIndexer(dataDir)

	err = indexer.AddIndex(&Pet{}, option.IndexByField("Kind"), "ID", "pets", "non_unique", nil, false)
	assert.NoError(t, err)

	pet1 := Pet{ID: "goefe-789", Kind: "Hog", Color: "Green", Name: "Dicky"}
	pet2 := Pet{ID: "xadaf-189", Kind: "Hog", Color: "Green", Name: "Ricky"}

	_, err = indexer.Add(pet1)
	assert.NoError(t, err)

	_, err = indexer.Add(pet2)
	assert.NoError(t, err)

	res, err := indexer.FindBy(Pet{}, NewField("Kind", "Hog"))
	assert.NoError(t, err)

	t.Log(res)

	_ = os.RemoveAll(dataDir)
}

func TestIndexer_Disk_AddWithAutoincrementIndex(t *testing.T) {
	dataDir, err := WriteIndexTestData(Data, "ID", "")
	assert.NoError(t, err)
	indexer := createDiskIndexer(dataDir)

	err = indexer.AddIndex(&User{}, option.IndexByField("UID"), "ID", "users", "autoincrement", &option.Bound{Lower: 5}, false)
	assert.NoError(t, err)

	res1, err := indexer.Add(Data["users"][0])
	assert.NoError(t, err)
	assert.Equal(t, "UID", res1[0].Field)
	assert.Equal(t, "5", path.Base(res1[0].Value))

	res2, err := indexer.Add(Data["users"][1])
	assert.NoError(t, err)
	assert.Equal(t, "UID", res2[0].Field)
	assert.Equal(t, "6", path.Base(res2[0].Value))

	resFindBy, err := indexer.FindBy(User{}, NewField("UID", "6"))
	assert.NoError(t, err)
	assert.Equal(t, "hijklmn-456", resFindBy[0])
	t.Log(resFindBy)

	_ = os.RemoveAll(dataDir)
}

func TestIndexer_Disk_DeleteWithNonUniqueIndex(t *testing.T) {
	dataDir, err := WriteIndexTestData(Data, "ID", "")
	assert.NoError(t, err)
	indexer := createDiskIndexer(dataDir)

	err = indexer.AddIndex(&Pet{}, option.IndexByField("Kind"), "ID", "pets", "non_unique", nil, false)
	assert.NoError(t, err)

	pet1 := Pet{ID: "goefe-789", Kind: "Hog", Color: "Green", Name: "Dicky"}
	pet2 := Pet{ID: "xadaf-189", Kind: "Hog", Color: "Green", Name: "Ricky"}

	_, err = indexer.Add(pet1)
	assert.NoError(t, err)

	_, err = indexer.Add(pet2)
	assert.NoError(t, err)

	err = indexer.Delete(pet2)
	assert.NoError(t, err)

	_ = os.RemoveAll(dataDir)
}

func TestIndexer_Disk_SearchWithNonUniqueIndex(t *testing.T) {
	dataDir, err := WriteIndexTestData(Data, "ID", "")
	assert.NoError(t, err)
	indexer := createDiskIndexer(dataDir)

	err = indexer.AddIndex(&Pet{}, option.IndexByField("Name"), "ID", "pets", "non_unique", nil, false)
	assert.NoError(t, err)

	pet1 := Pet{ID: "goefe-789", Kind: "Hog", Color: "Green", Name: "Dicky"}
	pet2 := Pet{ID: "xadaf-189", Kind: "Hog", Color: "Green", Name: "Ricky"}

	_, err = indexer.Add(pet1)
	assert.NoError(t, err)

	_, err = indexer.Add(pet2)
	assert.NoError(t, err)

	res, err := indexer.FindByPartial(pet2, "Name", "*ky")
	assert.NoError(t, err)

	t.Log(res)
	_ = os.RemoveAll(dataDir)
}

func TestIndexer_Disk_UpdateWithUniqueIndex(t *testing.T) {
	dataDir, err := WriteIndexTestData(Data, "ID", "")
	assert.NoError(t, err)
	indexer := createDiskIndexer(dataDir)

	err = indexer.AddIndex(&User{}, option.IndexByField("UserName"), "ID", "users", "unique", nil, false)
	assert.NoError(t, err)

	err = indexer.AddIndex(&User{}, option.IndexByField("Email"), "ID", "users", "unique", nil, false)
	assert.NoError(t, err)

	user1 := &User{ID: "abcdefg-123", UserName: "mikey", Email: "mikey@example.com"}
	user2 := &User{ID: "hijklmn-456", UserName: "frank", Email: "frank@example.com"}

	_, err = indexer.Add(user1)
	assert.NoError(t, err)

	_, err = indexer.Add(user2)
	assert.NoError(t, err)

	err = indexer.Update(user1, &User{
		ID:       "abcdefg-123",
		UserName: "mikey-new",
		Email:    "mikey@example.com",
	})
	assert.NoError(t, err)
	v, err1 := indexer.FindBy(&User{}, NewField("UserName", "mikey-new"))
	assert.NoError(t, err1)
	assert.Len(t, v, 1)
	v, err2 := indexer.FindBy(&User{}, NewField("UserName", "mikey"))
	assert.NoError(t, err2)
	assert.Len(t, v, 0)

	err1 = indexer.Update(&User{
		ID:       "abcdefg-123",
		UserName: "mikey-new",
		Email:    "mikey@example.com",
	}, &User{
		ID:       "abcdefg-123",
		UserName: "mikey-newest",
		Email:    "mikey-new@example.com",
	})
	assert.NoError(t, err1)
	fbUserName, err2 := indexer.FindBy(&User{}, NewField("UserName", "mikey-newest"))
	assert.NoError(t, err2)
	assert.Len(t, fbUserName, 1)
	fbEmail, err3 := indexer.FindBy(&User{}, NewField("Email", "mikey-new@example.com"))
	assert.NoError(t, err3)
	assert.Len(t, fbEmail, 1)

	_ = os.RemoveAll(dataDir)
}

func TestIndexer_Disk_UpdateWithNonUniqueIndex(t *testing.T) {
	dataDir, err := WriteIndexTestData(Data, "ID", "")
	assert.NoError(t, err)
	indexer := createDiskIndexer(dataDir)

	err = indexer.AddIndex(&Pet{}, option.IndexByField("Name"), "ID", "pets", "non_unique", nil, false)
	assert.NoError(t, err)

	pet1 := Pet{ID: "goefe-789", Kind: "Hog", Color: "Green", Name: "Dicky"}
	pet2 := Pet{ID: "xadaf-189", Kind: "Hog", Color: "Green", Name: "Ricky"}

	_, err = indexer.Add(pet1)
	assert.NoError(t, err)

	_, err = indexer.Add(pet2)
	assert.NoError(t, err)

	_ = os.RemoveAll(dataDir)
}

func TestQueryDiskImpl(t *testing.T) {
	dataDir, err := WriteIndexTestData(Data, "ID", "")
	assert.NoError(t, err)
	indexer := createDiskIndexer(dataDir)
	ctx := context.Background()

	err = indexer.AddIndex(&Account{}, option.IndexByField("OnPremisesSamAccountName"), "ID", "accounts", "non_unique", nil, false)
	assert.NoError(t, err)

	err = indexer.AddIndex(&Account{}, option.IndexByField("Mail"), "ID", "accounts", "non_unique", nil, false)
	assert.NoError(t, err)

	err = indexer.AddIndex(&Account{}, option.IndexByField("ID"), "ID", "accounts", "non_unique", nil, false)
	assert.NoError(t, err)

	acc := Account{
		ID:                       "ba5b6e54-e29d-4b2b-8cc4-0a0b958140d2",
		Mail:                     "spooky@skeletons.org",
		OnPremisesSamAccountName: "MrDootDoot",
	}

	_, err = indexer.Add(acc)
	assert.NoError(t, err)

	r, err := indexer.Query(ctx, &Account{}, "on_premises_sam_account_name eq 'MrDootDoot'") // this query will match both pets.
	assert.NoError(t, err)
	assert.Equal(t, []string{"ba5b6e54-e29d-4b2b-8cc4-0a0b958140d2"}, r)

	r, err = indexer.Query(ctx, &Account{}, "mail eq 'spooky@skeletons.org'") // this query will match both pets.
	assert.NoError(t, err)
	assert.Equal(t, []string{"ba5b6e54-e29d-4b2b-8cc4-0a0b958140d2"}, r)

	r, err = indexer.Query(ctx, &Account{}, "on_premises_sam_account_name eq 'MrDootDoot' or mail eq 'spooky@skeletons.org'") // this query will match both pets.
	assert.NoError(t, err)
	assert.Equal(t, []string{"ba5b6e54-e29d-4b2b-8cc4-0a0b958140d2"}, r)

	r, err = indexer.Query(ctx, &Account{}, "startswith(on_premises_sam_account_name,'MrDoo')") // this query will match both pets.
	assert.NoError(t, err)
	assert.Equal(t, []string{"ba5b6e54-e29d-4b2b-8cc4-0a0b958140d2"}, r)

	r, err = indexer.Query(ctx, &Account{}, "id eq 'ba5b6e54-e29d-4b2b-8cc4-0a0b958140d2' or on_premises_sam_account_name eq 'MrDootDoot'") // this query will match both pets.
	assert.NoError(t, err)
	assert.Equal(t, []string{"ba5b6e54-e29d-4b2b-8cc4-0a0b958140d2"}, r)

	_ = os.RemoveAll(dataDir)
}

func createDiskIndexer(dataDir string) *StorageIndexer {
	storage, err := metadata.NewDiskStorage(dataDir)
	if err != nil {
		return nil
	}

	return CreateIndexer(storage).(*StorageIndexer)
}
