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
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const timeoutMs = 30000

var revads = map[string]*Revad{}
var mutex = sync.Mutex{}
var port = 19000

func TestGprc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gprc Suite")
}

type shutdownFunc func() error

type Revad struct {
	GrpcAddress string
	shutdown    shutdownFunc
	references  int32
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
		fmt.Println("Reusing revad for config", config)
		revad := revads[config]
		revad.Use()
		return revad, nil
	}

	// Define a grpc address
	grpcAddress := fmt.Sprintf("localhost:%d", port)
	port += 1

	// Create a temporary root for this revad
	tmpRoot, err := ioutil.TempDir("", "reva-grpc-integration-tests-*-root")
	if err != nil {
		return nil, errors.Wrapf(err, "Could not create tmpdir")
	}
	newCfgPath := path.Join(tmpRoot, "config.toml")
	rawCfg, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not read config file")
	}
	cfg := string(rawCfg)
	cfg = strings.ReplaceAll(cfg, "{{root}}", tmpRoot)
	cfg = strings.ReplaceAll(cfg, "{{grpc_address}}", grpcAddress)
	err = ioutil.WriteFile(newCfgPath, []byte(cfg), 0600)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not write config file")
	}

	// Run revad
	cmd := exec.Command("../../../cmd/revad/revad", "-c", newCfgPath)
	err = cmd.Start()
	if err != nil {
		return nil, errors.Wrapf(err, "Could not start revad")
	}

	err = waitForPort(grpcAddress, "open")
	if err != nil {
		return nil, err
	}

	//even the port is open the service might not be available yet
	time.Sleep(1 * time.Second)

	revad := &Revad{
		GrpcAddress: grpcAddress,
		references:  1,
	}
	revad.shutdown = func() error {
		err := cmd.Process.Signal(os.Kill)
		if err != nil {
			return errors.Wrap(err, "Could not kill revad")
		}
		waitForPort(grpcAddress, "close")
		os.RemoveAll(tmpRoot)
		revads[config] = nil
		return nil
	}
	revads[config] = revad

	return revad, nil
}

func waitForPort(grpcAddress, expectedStatus string) error {
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
