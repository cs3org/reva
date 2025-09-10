// Copyright 2018-2025 CERN
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

package cache

import (
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type Cacheable = any

// Warmup is the interface to implement cache warmup strategies.
type GenericWarmup[T Cacheable] interface {
	GetInfos() ([]T, error)
}

type GenericCache[T Cacheable] interface {
	Get(key string) (T, error)
	GetKeys(keys []string) ([]T, error)
	Set(key string, info T) error
	SetWithExpire(key string, info T, expiration time.Duration) error
}

// ResourceInfo cache
type WarmupResourceInfo = GenericWarmup[*provider.ResourceInfo]
type ResourceInfoCache = GenericCache[*provider.ResourceInfo]

// Space cache
// We don't need to warm up this one
type SpaceInfoCache = GenericCache[*provider.StorageSpace]
