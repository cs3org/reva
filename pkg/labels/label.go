// Copyright 2018-2026 CERN
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

package labels

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// The CS3 labels API has no call yet for the labels of given resources. Until it has one,
// ListLabels takes the resources in its request opaque and returns their labels in its response opaque.
const (
	// OpaqueResources is a JSON list of resource IDs, each in the form <storage_id>!<opaque_id>.
	OpaqueResources = "resources"
	// OpaqueLabels is a JSON object from each requested resource ID to its labels.
	OpaqueLabels = "labels"
)

// Manager defines an interface for a labels manager.
type Manager interface {
	// List the unique labels that a user has attached to resources
	ListLabels(ctx context.Context) ([]string, error)
	// ListLabelsForResources returns the labels of the user for each given resource, keyed
	// on <storage_id>!<opaque_id>. A resource without labels has no key.
	ListLabelsForResources(ctx context.Context, ids []*provider.ResourceId) (map[string][]string, error)
	// List the resources which have a given label attached for a given user
	ListResourcesForLabel(ctx context.Context, label string) ([]*provider.ResourceId, error)
	// SetLabel qttaches a label to a resource for a user..
	SetLabel(ctx context.Context, label string, resourceId *provider.ResourceId) error
	// UnsetLabel removes a label from a resource for a user..
	UnsetLabel(ctx context.Context, label string, resourceId *provider.ResourceId) error
}
