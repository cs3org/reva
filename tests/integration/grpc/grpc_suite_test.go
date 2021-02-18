// Copyright 2018-2021 CERN
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

package grpc_test

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const grpcAddress = "localhost:19000"
const timeoutMs = 30000

var revads = map[string]*Revad{}
var mutex = sync.Mutex{}

func TestGprc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gprc Suite")
}

type shutdownFunc func() error

type Revad struct {
	shutdown   shutdownFunc
	references int32
}

func (r *Revad) Use() {
	atomic.AddInt32(&r.references, 1)
}

func (r *Revad) Cleanup() {
	references := atomic.AddInt32(&r.references, -1)
	if references == 0 {
		r.shutdown()
	}
}

func startRevad(config string) (*Revad, error) {
	mutex.Lock()
	defer mutex.Unlock()

	if revads[config] != nil {
		revad := revads[config]
		revad.Use()
		return revad, nil
	}

	cmd := exec.Command("../../../cmd/revad/revad", "-c", config)
	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("Could not start revad! ERROR: %v", err)
	}

	err = waitForPort("open")
	if err != nil {
		return nil, err
	}

	//even the port is open the service might not be available yet
	time.Sleep(1 * time.Second)

	revad := &Revad{
		references: 1,
	}
	revad.shutdown = func() error {
		err := cmd.Process.Signal(os.Kill)
		if err != nil {
			return fmt.Errorf("Could not kill revad! ERROR: %v", err)
		}
		waitForPort("close")
		return nil
	}

	return revad, nil
}

func waitForPort(expectedStatus string) error {
	if expectedStatus != "open" && expectedStatus != "close" {
		return errors.New("status can only be 'open' or 'close'")
	}
	timoutCounter := 0
	for timoutCounter <= timeoutMs {
		conn, err := net.Dial("tcp", grpcAddress)
		if err == nil {
			_ = conn.Close()
			if expectedStatus == "open" {
				break
			}
		} else if expectedStatus == "close" {
			break
		}

		time.Sleep(1 * time.Millisecond)
		timoutCounter++
	}
	return nil
}
