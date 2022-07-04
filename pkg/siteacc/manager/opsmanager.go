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

package manager

import (
	"strings"
	"sync"

	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/cs3org/reva/pkg/siteacc/data"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// OperatorsManager is responsible for all sites related tasks.
type OperatorsManager struct {
	conf *config.Configuration
	log  *zerolog.Logger

	storage data.Storage

	operators data.Operators

	mutex sync.RWMutex
}

func (mngr *OperatorsManager) initialize(storage data.Storage, conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	mngr.conf = conf

	if log == nil {
		return errors.Errorf("no logger provided")
	}
	mngr.log = log

	if storage == nil {
		return errors.Errorf("no storage provided")
	}
	mngr.storage = storage

	mngr.operators = make(data.Operators, 0, 32) // Reserve some space for operators
	mngr.readAllOperators()

	return nil
}

func (mngr *OperatorsManager) readAllOperators() {
	if ops, err := mngr.storage.ReadOperators(); err == nil {
		mngr.operators = *ops
	} else {
		// Just warn when not being able to read operators
		mngr.log.Warn().Err(err).Msg("error while reading operators")
	}
}

func (mngr *OperatorsManager) writeAllOperators() {
	if err := mngr.storage.WriteOperators(&mngr.operators); err != nil {
		// Just warn when not being able to write operators
		mngr.log.Warn().Err(err).Msg("error while writing operators")
	}
}

// GetOperator retrieves the operator with the given ID, creating it first if necessary.
func (mngr *OperatorsManager) GetOperator(id string, clone bool) (*data.Operator, error) {
	mngr.mutex.RLock()
	defer mngr.mutex.RUnlock()

	op, err := mngr.getOperator(id)
	if err != nil {
		return nil, err
	}

	if clone {
		op = op.Clone(false)
	}

	return op, nil
}

// FindOperator returns the operator specified by the ID if one exists.
func (mngr *OperatorsManager) FindOperator(id string) *data.Operator {
	op, _ := mngr.findOperator(id)
	return op
}

// FindSite returns the site specified by the ID if one exists.
func (mngr *OperatorsManager) FindSite(id string) (*data.Operator, *data.Site) {
	for _, op := range mngr.operators {
		for _, site := range op.Sites {
			if strings.EqualFold(site.ID, id) {
				return op, site
			}
		}
	}
	return nil, nil
}

// UpdateOperator updates the operator identified by the ID; if no such operator exists, one will be created first.
func (mngr *OperatorsManager) UpdateOperator(opData *data.Operator) error {
	mngr.mutex.Lock()
	defer mngr.mutex.Unlock()

	op, err := mngr.getOperator(opData.ID)
	if err != nil {
		return errors.Wrap(err, "operator to update not found")
	}

	if err := op.Update(opData); err == nil {
		mngr.storage.OperatorUpdated(op)
		mngr.writeAllOperators()
	} else {
		return errors.Wrap(err, "error while updating operator")
	}

	return nil
}

// CloneOperators retrieves all operators currently stored by cloning the data, thus avoiding race conflicts and making outside modifications impossible.
func (mngr *OperatorsManager) CloneOperators(eraseCredentials bool) data.Operators {
	mngr.mutex.RLock()
	defer mngr.mutex.RUnlock()

	clones := make(data.Operators, 0, len(mngr.operators))
	for _, op := range mngr.operators {
		clones = append(clones, op.Clone(eraseCredentials))
	}

	return clones
}

func (mngr *OperatorsManager) getOperator(id string) (*data.Operator, error) {
	op, err := mngr.findOperator(id)
	if op == nil {
		op, err = mngr.createOperator(id)
	}
	return op, err
}

func (mngr *OperatorsManager) createOperator(id string) (*data.Operator, error) {
	op, err := data.NewOperator(id)
	if err != nil {
		return nil, errors.Wrap(err, "error while creating operator")
	}
	mngr.operators = append(mngr.operators, op)
	mngr.storage.OperatorAdded(op)
	mngr.writeAllOperators()
	return op, nil
}

func (mngr *OperatorsManager) findOperator(id string) (*data.Operator, error) {
	if len(id) == 0 {
		return nil, errors.Errorf("no search ID specified")
	}

	op := mngr.findOperatorByPredicate(func(op *data.Operator) bool { return strings.EqualFold(op.ID, id) })
	if op != nil {
		return op, nil
	}

	return nil, errors.Errorf("no operator found matching the specified ID")
}

func (mngr *OperatorsManager) findOperatorByPredicate(predicate func(operator *data.Operator) bool) *data.Operator {
	for _, op := range mngr.operators {
		if predicate(op) {
			return op
		}
	}
	return nil
}

// NewOperatorsManager creates a new operators manager instance.
func NewOperatorsManager(storage data.Storage, conf *config.Configuration, log *zerolog.Logger) (*OperatorsManager, error) {
	mngr := &OperatorsManager{}
	if err := mngr.initialize(storage, conf, log); err != nil {
		return nil, errors.Wrap(err, "unable to initialize the operators manager")
	}
	return mngr, nil
}
