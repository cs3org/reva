// Copyright 2018-2023 CERN
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

package accumulator

import (
	"errors"
	"time"

	"github.com/rs/zerolog"
)

// Accumulator gathers items arriving spaced in time and groups them.
type Accumulator[T any] struct {
	started          bool
	timeout          time.Duration
	timeoutChan      chan bool
	timeoutResetChan chan bool
	maxSize          int
	Input            chan T
	pool             []T
	log              *zerolog.Logger
}

// New creates a new accumulator. An Accumulator gathers items arriving spaced
// in time and groups them.
//
// The main parameters are timeout and maxSize, determining the limits for the
// accumulator.
//
// An accumulator is started with the start method, which takes fn, a func([]T)
// argument that will be run every time the limit parameters are reached. After
// running fn, the accumulator pool is emptied.
//
// Items are put into the accumulator using the <-input channel, making it
// thread-safe.
func New[T any](timeout time.Duration, maxSize int, log *zerolog.Logger) *Accumulator[T] {
	if timeout == 0 {
		timeout = time.Duration(60) * time.Second
		log.Warn().Msgf("timeout must be a positive duration greater than zero, using default (%d)", timeout)
	}

	if maxSize == 0 {
		maxSize = 100
		log.Warn().Msgf("maxSize must be a positive integer greater than zero, using default (%d)", maxSize)
	}

	input := make(chan T)
	accumulator := &Accumulator[T]{
		timeout:          timeout,
		timeoutResetChan: make(chan bool, 1),
		maxSize:          maxSize,
		Input:            input,
		log:              log,
	}

	return accumulator
}

func (a *Accumulator[T]) startTimeout() {
	if !a.started {
		a.started = true
		a.timeoutChan = make(chan bool)
		go func() {
			select {
			case <-a.timeoutResetChan:
				a.timeoutChan = nil
			case <-time.After(a.timeout):
				a.timeoutChan <- true
				a.timeoutChan = nil
			}
			a.started = false
		}()
	}
}

// Start starts the accumulator.
//
// This does not mean the timer will start running. That happens once the first
// item arrives through the <-input channel. Once the time reaches the timeout
// or the max size of the accumulator is reached, fn will be run with the slice
// of items currently in the accumulator.
func (a *Accumulator[T]) Start(fn func([]T)) error {
	if fn == nil {
		return errors.New("fn must be a callback function")
	}

	go func() {
		for {
			select {
			case i := <-a.Input:
				a.startTimeout()
				a.pool = append(a.pool, i)

				if len(a.pool) >= a.maxSize {
					fn(a.pool)
					a.pool = nil
					a.timeoutResetChan <- true
					a.timeoutChan = nil
				}
			case <-a.timeoutChan:
				if len(a.pool) > 0 {
					fn(a.pool)
					a.pool = nil
				}
			}
		}
	}()

	return nil
}
