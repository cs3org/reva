// Copyright 2018-2019 CERN
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

// this file is a fork of https://github.com/tus/tusd/tree/master/pkg/handler/composer.go
// TODO remove when PRs have been merged upstream

package handler

// StoreComposer represents a composable data store. It consists of the core
// data store and optional extensions. Please consult the package's overview
// for a more detailed introduction in how to use this structure.
type StoreComposer struct {
	Core DataStore

	UsesCreator        bool
	Creator            CreatorDataStore
	UsesTerminater     bool
	Terminater         TerminaterDataStore
	UsesLocker         bool
	Locker             Locker
	UsesConcater       bool
	Concater           ConcaterDataStore
	UsesLengthDeferrer bool
	LengthDeferrer     LengthDeferrerDataStore
}

// NewStoreComposer creates a new and empty store composer.
func NewStoreComposer() *StoreComposer {
	return &StoreComposer{}
}

// Capabilities returns a string representing the provided extensions in a
// human-readable format meant for debugging.
func (store *StoreComposer) Capabilities() string {
	str := "Core: "

	if store.Core != nil {
		str += "✓"
	} else {
		str += "✗"
	}

	str += ` Creator: `
	if store.UsesCreator {
		str += "✓"
	} else {
		str += "✗"
	}
	str += ` Terminater: `
	if store.UsesTerminater {
		str += "✓"
	} else {
		str += "✗"
	}
	str += ` Locker: `
	if store.UsesLocker {
		str += "✓"
	} else {
		str += "✗"
	}
	str += ` Concater: `
	if store.UsesConcater {
		str += "✓"
	} else {
		str += "✗"
	}
	str += ` LengthDeferrer: `
	if store.UsesLengthDeferrer {
		str += "✓"
	} else {
		str += "✗"
	}

	return str
}

// UseCore will set the used core data store.
func (store *StoreComposer) UseCore(core DataStore) {
	store.Core = core
}

// UseCreator will set the used creator data store. If the argument is nil, the
// property will be unset.
func (store *StoreComposer) UseCreator(ext CreatorDataStore) {
	store.UsesCreator = ext != nil
	store.Creator = ext
}

// UseTerminater will set the used terminator data store. If the argument is nil, the
// property will be unset.
func (store *StoreComposer) UseTerminater(ext TerminaterDataStore) {
	store.UsesTerminater = ext != nil
	store.Terminater = ext
}

// UseLocker will set the used locker. If the argument is nil, the
// property will be unset.
func (store *StoreComposer) UseLocker(ext Locker) {
	store.UsesLocker = ext != nil
	store.Locker = ext
}

// UseConcater will set the used concater data store. If the argument is nil, the
// property will be unset.
func (store *StoreComposer) UseConcater(ext ConcaterDataStore) {
	store.UsesConcater = ext != nil
	store.Concater = ext
}

// UseLengthDeferrer will set the used length deferrer data store. If the argument is nil, the
// property will be unset.
func (store *StoreComposer) UseLengthDeferrer(ext LengthDeferrerDataStore) {
	store.UsesLengthDeferrer = ext != nil
	store.LengthDeferrer = ext
}

// Composable is the interface that a struct needs to implement to be composable by this composer
type Composable interface {
	UseIn(composer *StoreComposer)
}
