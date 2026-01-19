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

// Manager defines an interface for a labels manager.
type Manager interface {
	// List the unique labels that a user has attached to resources
	ListLabels(ctx context.Context) ([]string, error)
	// List the resources which have a given label attached for a given user
	ListResourcesForLabel(ctx context.Context, label string) ([]*provider.ResourceId, error)
	// SetLabel qttaches a label to a resource for a user..
	SetLabel(ctx context.Context, label string, resourceId *provider.ResourceId) error
	// UnsetLabel removes a label from a resource for a user..
	UnsetLabel(ctx context.Context, label string, resourceId *provider.ResourceId) error
}
