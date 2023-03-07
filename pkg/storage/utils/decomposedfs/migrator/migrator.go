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

package migrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/rs/zerolog"
)

var allMigrations = []string{"0001", "0002"}

type migrationFunc func() error
type migrationState struct {
	state   string
	message string
}
type migrationStates map[string]migrationState

// Migrator runs migrations on an existing decomposedfs
type Migrator struct {
	lu     *lookup.Lookup
	states migrationStates
	log    *zerolog.Logger
}

// New returns a new Migrator instance
func New(lu *lookup.Lookup, log *zerolog.Logger) Migrator {
	return Migrator{
		lu:  lu,
		log: log,
	}
}

/**
 * RunMigrations runs all migrations in sequence.
 * Note this sequence must not be changed or it might
 * damage existing decomposed fs.
 */
func (m *Migrator) RunMigrations() error {
	err := m.readStates()
	if err != nil {
		return err
	}

	for _, migration := range allMigrations {
		if s, ok := m.states[migration]; !ok || s.state != "succeeded" {
			if s.state != "" {
				m.log.Info().Msg("Re-running migration " + migration + "...")
			} else {
				m.log.Info().Msg("Running migration " + migration + "...")
			}
			migrateMethod := reflect.ValueOf(m).MethodByName("migration" + migration)
			v := migrateMethod.Call(nil)
			err := v[0].Interface().(error)
			if err != nil {
				m.log.Error().Err(err).Msg("migration " + migration + " failed")
				s.state = "failed"
				s.message = err.Error()
				m.writeStates()
				return err
			}
		}
	}
	return nil
}

func (m *Migrator) readStates() error {
	d, err := os.ReadFile(filepath.Join(m.lu.InternalRoot(), ".migrations"))
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	if len(d) > 0 {
		json.Unmarshal(d, &m.states)
	}

	return nil
}

func (m *Migrator) writeStates() error {
	d, err := json.Marshal(m.states)
	if err != nil {
		m.log.Error().Err(err).Msg("could not marshal migration states")
		return nil
	}
	return os.WriteFile(filepath.Join(m.lu.InternalRoot(), ".migrations"), d, 0600)
}
