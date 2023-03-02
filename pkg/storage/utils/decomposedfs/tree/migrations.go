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

package tree

import (
	"io"
	"os"
	"path/filepath"

	"github.com/cs3org/reva/v2/pkg/logger"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
)

/**
 * This function runs all migrations in sequence.
 * Note this sequence must not be changed or it might
 * damage existing decomposed fs.
 */
func (t *Tree) runMigrations() error {
	if err := t.migration0001Nodes(); err != nil {
		return err
	}
	return t.migration0002SpaceTypes()
}

func (t *Tree) migration0001Nodes() error {
	// create spaces folder and iterate over existing nodes to populate it
	nodesPath := filepath.Join(t.root, "nodes")
	fi, err := os.Stat(nodesPath)
	if err == nil && fi.IsDir() {

		f, err := os.Open(nodesPath)
		if err != nil {
			return err
		}
		nodes, err := f.Readdir(0)
		if err != nil {
			return err
		}

		for _, n := range nodes {
			nodePath := filepath.Join(nodesPath, n.Name())

			attr, err := t.lookup.MetadataBackend().Get(nodePath, prefixes.ParentidAttr)
			if err == nil && attr == node.RootID {
				if err := t.moveNode(n.Name(), n.Name()); err != nil {
					logger.New().Error().Err(err).
						Str("space", n.Name()).
						Msg("could not move space")
					continue
				}
				t.linkSpaceNode("personal", n.Name())
			}
		}
		// TODO delete nodesPath if empty
	}
	return nil
}

func (t *Tree) migration0002SpaceTypes() error {
	spaceTypesPath := filepath.Join(t.root, "spacetypes")
	fi, err := os.Stat(spaceTypesPath)
	if err == nil && fi.IsDir() {

		f, err := os.Open(spaceTypesPath)
		if err != nil {
			return err
		}
		spaceTypes, err := f.Readdir(0)
		if err != nil {
			return err
		}

		for _, st := range spaceTypes {
			err := t.moveSpaceType(st.Name())
			if err != nil {
				logger.New().Error().Err(err).
					Str("space", st.Name()).
					Msg("could not move space")
				continue
			}
		}

		// delete spacetypespath
		d, err := os.Open(spaceTypesPath)
		if err != nil {
			logger.New().Error().Err(err).
				Str("spacetypesdir", spaceTypesPath).
				Msg("could not open spacetypesdir")
			return nil
		}
		defer d.Close()
		_, err = d.Readdirnames(1) // Or f.Readdir(1)
		if err == io.EOF {
			// directory is empty we can delete
			err := os.Remove(spaceTypesPath)
			if err != nil {
				logger.New().Error().Err(err).
					Str("spacetypesdir", d.Name()).
					Msg("could not delete")
			}
		} else {
			logger.New().Error().Err(err).
				Str("spacetypesdir", d.Name()).
				Msg("could not delete, not empty")
		}
	}
	return nil
}
