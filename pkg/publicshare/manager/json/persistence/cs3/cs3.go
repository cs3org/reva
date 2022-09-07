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

package cs3

import (
	"context"
	"encoding/json"
	"fmt"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/json/persistence"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
)

// FIXME the in memory data structure in the json manager is ... awkward. it does not even use a map per space ... weird
type cs3 struct {
	gwa, pa, suid, suidp, maak string
	initialized                bool
	s                          metadata.Storage
	client                     gateway.GatewayAPIClient
}

// New returns a new Cache instance
func New(gwa, pa, suid, suidp, maak string) persistence.Persistence {
	return &cs3{
		gwa:   gwa,
		pa:    pa,
		suid:  suid,
		suidp: suidp,
		maak:  maak,
	}
}

func (p *cs3) InitDB() error {
	if p.initialized { // check if initialization happened while grabbing the lock
		return nil
	}
	var err error
	p.s, err = metadata.NewCS3Storage(p.gwa, p.pa, p.suid, p.suidp, p.maak)
	if err != nil {
		return err
	}

	err = p.s.Init(context.Background(), "jsoncs3-public-share-manager-metadata")
	if err != nil {
		return err
	}
	if err := p.s.MakeDirIfNotExist(context.Background(), "publicshares"); err != nil {
		return err
	}
	// stat and create publicshares.json?
	if _, err := p.s.Stat(context.TODO(), "publicshares.json"); err != nil {
		/*err*/ p.s.Upload(context.TODO(), metadata.UploadRequest{
			Path:    "publicshares.json",
			Content: []byte("{}"),
		})
	}
	// or introduce a PersistWithCTX(ctx context.Context, db map[string]interface{}, ifUnchangedSince time.Time)
	// and ReadWithCTX(ctx context.Context, ifModifiedSince time.Time) (db map[string]interface{}, error)
	// or go micro store interface?
	p.initialized = true

	return nil
}

func (p *cs3) ReadDB() (map[string]interface{}, error) {
	if !p.initialized {
		return nil, fmt.Errorf("not initialized")
	}
	db := map[string]interface{}{}
	readBytes, err := p.s.SimpleDownload(context.TODO(), "publicshares.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(readBytes, &db); err != nil {
		return nil, err
	}
	return db, nil
}

func (p *cs3) WriteDB(db map[string]interface{}) error {
	if !p.initialized {
		return fmt.Errorf("not initialized")
	}
	dbAsJSON, err := json.Marshal(db)
	if err != nil {
		return err
	}

	return p.s.SimpleUpload(context.TODO(), "publicshares.json", dbAsJSON)
}
