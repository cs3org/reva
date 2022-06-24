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

package postprocessing_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cs3org/reva/v2/pkg/utils/postprocessing"
	"github.com/test-go/testify/require"
)

var (
	// should be long enough so running processes can be tracked but obviously also as short as possible :)
	_waitTime    = 500 * time.Millisecond
	_minWaitTime = time.Millisecond
)

func SuccessAfter(t time.Duration) func() error {
	return func() error {
		time.Sleep(t)
		return nil
	}
}

func FailureAfter(t time.Duration) func() error {
	return func() error {
		time.Sleep(t)
		return errors.New("epic fail")
	}
}

// concurrent boolean
type Cbool struct {
	b bool
	l *sync.Mutex
}

func Bool() *Cbool {
	return &Cbool{b: false, l: &sync.Mutex{}}
}

func (b *Cbool) Set(v bool) {
	b.l.Lock()
	defer b.l.Unlock()
	b.b = v
}

func (b *Cbool) Get() bool {
	b.l.Lock()
	defer b.l.Unlock()
	return b.b
}

func Test_ItRunsStepsAsync(t *testing.T) {
	stepdone := Bool()
	pp := postprocessing.Postprocessing{
		Steps: []postprocessing.Step{
			postprocessing.NewStep("stepA", FailureAfter(_waitTime), func(error) {
				stepdone.Set(true)
			}),
		},
	}

	err := pp.Start()
	require.NoError(t, err)
	require.False(t, stepdone.Get())
}

func Test_ItSyncsIfConfigured(t *testing.T) {
	stepdone := Bool()
	pp := postprocessing.Postprocessing{
		Steps: []postprocessing.Step{
			postprocessing.NewStep("stepA", FailureAfter(_waitTime), func(error) {
				stepdone.Set(true)
			}),
		},
		WaitFor: []string{"stepA"},
	}

	err := pp.Start()
	require.Error(t, err)
	require.True(t, stepdone.Get())
}

func Test_ItRunsStepsInParallel(t *testing.T) {
	astarted, afinished := Bool(), Bool()
	bstarted, bfinished := Bool(), Bool()
	pp := postprocessing.Postprocessing{
		Steps: []postprocessing.Step{
			postprocessing.NewStep("stepA", func() error {
				astarted.Set(true)
				time.Sleep(_waitTime)
				return nil
			}, func(error) {
				afinished.Set(true)
			}),
			postprocessing.NewStep("stepB", func() error {
				bstarted.Set(true)
				time.Sleep(_waitTime)
				return nil
			}, func(error) {
				bfinished.Set(false)
			}),
		},
	}

	err := pp.Start()
	require.NoError(t, err)
	time.Sleep(_minWaitTime) // wait till processes have started
	require.True(t, astarted.Get())
	require.True(t, bstarted.Get())
	require.False(t, afinished.Get())
	require.False(t, bfinished.Get())
}

func Test_ItWaitsForSpecificSteps(t *testing.T) {
	stepdone := Bool()
	pp := postprocessing.Postprocessing{
		Steps: []postprocessing.Step{
			postprocessing.NewStep("stepA", func() error {
				time.Sleep(_waitTime)
				stepdone.Set(true)
				return nil
			}, nil),
			postprocessing.NewStep("stepB", func() error {
				if !stepdone.Get() {
					return errors.New("step not done")
				}
				return nil
			}, nil, "stepA"),
		},
		WaitFor: []string{"stepB"},
	}

	err := pp.Start()
	require.NoError(t, err)
}

func Test_ItCollectsStepResults(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	var results map[string]error
	pp := postprocessing.Postprocessing{
		Steps: []postprocessing.Step{
			postprocessing.NewStep("stepA", func() error {
				time.Sleep(_waitTime)
				return errors.New("stepA failed")
			}, nil),
			postprocessing.NewStep("stepB", SuccessAfter(_waitTime), nil),
			postprocessing.NewStep("stepC", func() error {
				time.Sleep(_waitTime)
				return errors.New("stepC failed")
			}, nil),
		},
		Finish: func(m map[string]error) {
			results = m
			wg.Done()
		},
	}

	err := pp.Start()
	require.NoError(t, err)
	wg.Wait()
	e, ok := results["stepA"]
	require.True(t, ok)
	require.Error(t, e)
	require.Equal(t, "stepA failed", e.Error())
	e, ok = results["stepB"]
	require.True(t, ok)
	require.NoError(t, e)
	e, ok = results["stepC"]
	require.True(t, ok)
	require.Error(t, e)
	require.Equal(t, "stepC failed", e.Error())
}
