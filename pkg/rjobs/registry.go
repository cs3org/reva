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

package rjobs

import (
	"sync"

	"github.com/pkg/errors"
)

// registry holds the jobs registered in the process. Periodic jobs are
// registered as ready-to-run values (the Run closure carries the
// dependencies), while on-demand jobs are registered as constructors that the
// jobs service builds from config. The two kinds share one name space so a
// collision is reported at registration time.
type registry struct {
	mu       sync.Mutex
	periodic map[string]Periodic
	onDemand map[string]NewJob
}

var reg = &registry{
	periodic: make(map[string]Periodic),
	onDemand: make(map[string]NewJob),
}

// RegisterPeriodic registers a periodic job. It can be called at any time:
// at init for self-contained jobs, at the owning component's construction
// time with a closure capturing live dependencies, or even after the runner
// has started, in which case the job is picked up on the scheduler's next
// pass. It validates the job and returns an error if the name is already
// taken or the spec is invalid.
func RegisterPeriodic(p Periodic) error {
	if err := validatePeriodic(p); err != nil {
		return err
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()

	if _, ok := reg.periodic[p.Name]; ok {
		return errors.Errorf("rjobs: periodic job %q already registered", p.Name)
	}
	if _, ok := reg.onDemand[p.Name]; ok {
		return errors.Errorf("rjobs: job %q already registered as on-demand", p.Name)
	}
	reg.periodic[p.Name] = p
	return nil
}

// RegisterOnDemand registers an on-demand job constructor under the given
// name.
func RegisterOnDemand(name string, newFunc NewJob) error {
	if name == "" {
		return errors.New("rjobs: on-demand job name must not be empty")
	}
	if newFunc == nil {
		return errors.Errorf("rjobs: on-demand job %q has nil constructor", name)
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()

	if _, ok := reg.onDemand[name]; ok {
		return errors.Errorf("rjobs: on-demand job %q already registered", name)
	}
	if _, ok := reg.periodic[name]; ok {
		return errors.Errorf("rjobs: job %q already registered as periodic", name)
	}
	reg.onDemand[name] = newFunc
	return nil
}

func validatePeriodic(p Periodic) error {
	if p.Name == "" {
		return errors.New("rjobs: periodic job name must not be empty")
	}
	if p.Run == nil {
		return errors.Errorf("rjobs: periodic job %q has nil Run", p.Name)
	}
	if p.Scope != ScopeAllNodes && p.Scope != ScopeLeader {
		return errors.Errorf("rjobs: periodic job %q must declare a valid Scope", p.Name)
	}
	if _, err := ParseSchedule(p.Schedule); err != nil {
		return errors.Wrapf(err, "rjobs: periodic job %q", p.Name)
	}
	return nil
}

// registeredPeriodic returns a copy of the currently registered periodic
// jobs. The runner reads it whenever it needs the current job set, so it
// always observes the latest registrations.
func registeredPeriodic() []Periodic {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	out := make([]Periodic, 0, len(reg.periodic))
	for _, p := range reg.periodic {
		out = append(out, p)
	}
	return out
}

// lookupPeriodic returns the periodic job registered under name, if any.
func lookupPeriodic(name string) (Periodic, bool) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	p, ok := reg.periodic[name]
	return p, ok
}

// lookupOnDemand returns the constructor registered for name, if any.
func lookupOnDemand(name string) (NewJob, bool) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	f, ok := reg.onDemand[name]
	return f, ok
}

// RegisteredQueueJobNames returns the names of every job that flows through the
// durable queue in this process: all on-demand jobs plus the leader-scoped
// periodic jobs. All-nodes periodic jobs are excluded since they never touch
// the queue. The jobs service passes this to the store so a process only
// subscribes to (and therefore only claims) the jobs it has registered.
func RegisteredQueueJobNames() []string {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	out := make([]string, 0, len(reg.onDemand)+len(reg.periodic))
	for name := range reg.onDemand {
		out = append(out, name)
	}
	for name, p := range reg.periodic {
		if p.Scope == ScopeLeader {
			out = append(out, name)
		}
	}
	return out
}
