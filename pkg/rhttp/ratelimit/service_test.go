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

package ratelimit

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestServiceConfigDefaults(t *testing.T) {
	c := ServiceConfig{}
	c.ApplyDefaults()
	if c.UnauthRateLimit != DefaultRequestsPerMinute || c.UnauthRateLimitBurst != DefaultBurst {
		t.Fatalf("defaults not applied: %+v", c)
	}
	c = ServiceConfig{UnauthRateLimit: 5, UnauthRateLimitBurst: 2}
	c.ApplyDefaults()
	if c.UnauthRateLimit != 5 || c.UnauthRateLimitBurst != 2 {
		t.Fatalf("explicit values overridden: %+v", c)
	}
}

func TestServiceConfigNewLimiter(t *testing.T) {
	log := zerolog.Nop()

	c := ServiceConfig{UnauthRateLimit: -1}
	c.ApplyDefaults()
	l, err := c.NewLimiter(&log)
	if err != nil || l != nil {
		t.Fatalf("negative rate: got (%v, %v), want disabled limiter", l, err)
	}

	c = ServiceConfig{TrustedProxyCIDRs: []string{"not-a-cidr"}}
	c.ApplyDefaults()
	if _, err := c.NewLimiter(&log); err == nil {
		t.Fatal("invalid trusted proxy CIDR accepted")
	}

	c = ServiceConfig{}
	c.ApplyDefaults()
	l, err = c.NewLimiter(&log)
	if err != nil || l == nil {
		t.Fatalf("got (%v, %v), want a limiter", l, err)
	}
	l.Close()
}
