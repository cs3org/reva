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

package data

import (
	"encoding/json"
	"sort"

	"github.com/cs3org/reva/pkg/mentix/utils/network"
	"github.com/pkg/errors"
)

// OperatorInformation holds the most basic information about an operator and its sites.
type OperatorInformation struct {
	ID   string
	Name string

	Sites []SiteInformation
}

// QueryAvailableOperators uses Mentix to query a list of all available operators and sites.
func QueryAvailableOperators(mentixHost, dataEndpoint string) ([]OperatorInformation, error) {
	mentixURL, err := network.GenerateURL(mentixHost, dataEndpoint, network.URLParams{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate Mentix URL")
	}

	data, err := network.ReadEndpoint(mentixURL, nil, true)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read the Mentix endpoint")
	}

	// Decode the data into a simplified, reduced data type
	type operatorsData struct {
		Operators []OperatorInformation
	}
	operators := operatorsData{}
	if err := json.Unmarshal(data, &operators); err != nil {
		return nil, errors.Wrap(err, "error while decoding the JSON data")
	}

	sort.Slice(operators.Operators, func(i, j int) bool {
		return operators.Operators[i].Name < operators.Operators[j].Name
	})
	return operators.Operators, nil
}

// QueryOperatorName uses Mentix to query the name of an operator given by its ID.
func QueryOperatorName(opID string, mentixHost, dataEndpoint string) (string, error) {
	ops, err := QueryAvailableOperators(mentixHost, dataEndpoint)
	if err != nil {
		return "", err
	}

	for _, op := range ops {
		if op.ID == opID {
			return op.Name, nil
		}
	}

	return "", errors.Errorf("no operator with ID %v found", opID)
}

// QueryOperatorSites uses Mentix to query the sites associated with the specified operator.
func QueryOperatorSites(opID string, mentixHost, dataEndpoint string) ([]string, error) {
	ops, err := QueryAvailableOperators(mentixHost, dataEndpoint)
	if err != nil {
		return []string{}, err
	}

	for _, op := range ops {
		if op.ID == opID {
			var sites []string
			for _, site := range op.Sites {
				sites = append(sites, site.ID)
			}
			return sites, nil
		}
	}

	return []string{}, errors.Errorf("no operator with ID %v found", opID)
}
