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

package postprocessing

import (
	"sync"
)

// StepFunc is a postprocessing step function
type StepFunc func() error

// Step contains information about one step
type Step struct {
	Step         StepFunc
	Alias        string
	Requires     []string
	HandleResult func(error)
	wg           *sync.WaitGroup
}

// StepResult contains information about the result of one step
type StepResult struct {
	Alias string
	Error error
}

// NewStep creates a Step to be used for Postprocessing
func NewStep(alias string, step StepFunc, handleResult func(error), requires ...string) Step {
	return Step{
		Step:         step,
		Alias:        alias,
		Requires:     requires,
		HandleResult: handleResult,
		wg:           &sync.WaitGroup{},
	}
}

// Postprocessing holds information on how to handle file postprocessing
type Postprocessing struct {
	// Steps will wait BEFORE execution until the condition are met then run async
	Steps []Step
	// WaitFor contains a list of steps to wait for before return
	WaitFor []string
	// Will be called when all steps are finished. Gets a map[string]error showing the results
	Finish func(map[string]error)
}

// Start starts postprocessing
func (pp Postprocessing) Start() error {
	ch := make(chan StepResult)
	for _, sd := range pp.Steps {
		pp.startStep(sd, ch)
	}

	return pp.Process(ch)
}

// Process collects results of the post processing
func (pp Postprocessing) Process(ch <-chan StepResult) error {
	finished := make(map[string]error, len(pp.Steps))
	waitFor := make(map[string]bool, len(pp.WaitFor))

	wg := sync.WaitGroup{}
	for _, w := range pp.WaitFor {
		wg.Add(1)
		waitFor[w] = true
	}

	go func() {
		for _, s := range pp.Steps {
			for {
				if err, ok := finished[s.Alias]; ok {
					if s.HandleResult != nil {
						s.HandleResult(err)
					}
					if waitFor[s.Alias] {
						wg.Done()
					}
					break
				}
				sr := <-ch
				finished[sr.Alias] = sr.Error
			}
		}

		if pp.Finish != nil {
			pp.Finish(finished)
		}
	}()

	wg.Wait()
	// return first waitfor error if it occurred
	for _, w := range pp.WaitFor {
		if err := finished[w]; err != nil {
			return err
		}
	}

	return nil
}

// will run the step in separate go-routine after checking dependencies
func (pp Postprocessing) startStep(s Step, ch chan<- StepResult) {
	// check if the step is needed for some other step
	var wgs []*sync.WaitGroup
	for _, cs := range pp.Steps {
		for _, r := range cs.Requires {
			if r == s.Alias {
				cs.wg.Add(1)
				wgs = append(wgs, cs.wg)
			}
		}
	}

	// run in separate go-rountine
	go func(sd Step) {
		sd.wg.Wait()
		err := sd.Step()
		for _, wg := range wgs {
			wg.Done()
		}
		ch <- StepResult{Alias: sd.Alias, Error: err}
	}(s)
}
