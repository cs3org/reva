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

package sync

import (
	"fmt"
	"runtime"
	"testing"
)

func HammerMutex(m *NamedRWMutex, loops int, c chan bool) {
	for i := 0; i < loops; i++ {
		id := fmt.Sprintf("%v", i)
		m.Lock(id)
		m.Unlock(id)
	}
	c <- true
}

func TestNamedRWMutex(t *testing.T) {
	if n := runtime.SetMutexProfileFraction(1); n != 0 {
		t.Logf("got mutexrate %d expected 0", n)
	}
	defer runtime.SetMutexProfileFraction(0)
	m := NewNamedRWMutex()
	c := make(chan bool)
	r := 10

	for i := 0; i < r; i++ {
		go HammerMutex(&m, 2000, c)
	}
	for i := 0; i < r; i++ {
		<-c
	}
}
