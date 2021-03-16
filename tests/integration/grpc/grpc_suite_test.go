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
	"testing"
	"time"

	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const timeoutMs = 30000

var mutex = sync.Mutex{}
var port = 19000

func TestGprc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gprc Suite")
}

type cleanupFunc func(bool) error

// Revad represents a running revad process
type Revad struct {
	TmpRoot     string      // Temporary directory on disk. Will be cleaned up by the Cleanup func.
	GrpcAddress string      // Address of the grpc service
	Cleanup     cleanupFunc // Function to kill the process and cleanup the temp. root. If the given parameter is true the files will be kept to make debugging failures easier.
}

// stardRevads takes a list of revad configuration files plus a map of
// variables that need to be substituted in them and starts them.
//
// A unique port is assigned to each spawned instance.
// Placeholders in the config files can be replaced the variables from the
// `variables` map, e.g. the config
//
//   redis = "{{redis_address}}"
//
// and the variables map
//
//   variables = map[string]string{"redis_address": "localhost:6379"}
//
// will result in the config
//
//   redis = "localhost:6379"
//
// Special variables are created for the revad addresses, e.g. having a
// `storage` and a `users` revad will make `storage_address` and
// `users_address` available wit the dynamically assigned ports so that
// the services can be made available to each other.
func startRevads(configs map[string]string, variables map[string]string) (map[string]*Revad, error) {
	mutex.Lock()
	defer mutex.Unlock()

	revads := map[string]*Revad{}
	addresses := map[string]string{}
	for name := range configs {
		addresses[name] = fmt.Sprintf("localhost:%d", port)
		port++
	}

	for name, config := range configs {
		ownAddress := addresses[name]

		// Create a temporary root for this revad
		tmpRoot, err := ioutil.TempDir("", "reva-grpc-integration-tests-*-root")
		if err != nil {
			return nil, errors.Wrapf(err, "Could not create tmpdir")
		}
		newCfgPath := path.Join(tmpRoot, "config.toml")
		rawCfg, err := ioutil.ReadFile(path.Join("fixtures", config))
		if err != nil {
			return nil, errors.Wrapf(err, "Could not read config file")
		}
		cfg := string(rawCfg)
		cfg = strings.ReplaceAll(cfg, "{{root}}", tmpRoot)
		cfg = strings.ReplaceAll(cfg, "{{grpc_address}}", ownAddress)
		for v, value := range variables {
			cfg = strings.ReplaceAll(cfg, "{{"+v+"}}", value)
		}
		for name, address := range addresses {
			cfg = strings.ReplaceAll(cfg, "{{"+name+"_address}}", address)
		}
		err = ioutil.WriteFile(newCfgPath, []byte(cfg), 0600)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not write config file")
		}

		// Run revad
		cmd := exec.Command("../../../cmd/revad/revad", "-c", newCfgPath)

		outfile, err := os.Create(path.Join(tmpRoot, name+"-out.log"))
		if err != nil {
			panic(err)
		}
		defer outfile.Close()
		cmd.Stdout = outfile
		cmd.Stderr = outfile

		err = cmd.Start()
		if err != nil {
			return nil, errors.Wrapf(err, "Could not start revad")
		}

		err = waitForPort(ownAddress, "open")
		if err != nil {
			return nil, err
		}

		// even the port is open the service might not be available yet
		time.Sleep(1 * time.Second)

		revad := &Revad{
			TmpRoot:     tmpRoot,
			GrpcAddress: ownAddress,
			Cleanup: func(keepLogs bool) error {
				err := cmd.Process.Signal(os.Kill)
				if err != nil {
					return errors.Wrap(err, "Could not kill revad")
				}
				_ = waitForPort(ownAddress, "close")
				if keepLogs {
					fmt.Println("Test failed, keeping root", tmpRoot, "around for debugging")
				} else {
					os.RemoveAll(tmpRoot)
				}
				return nil
			},
		}
		revads[name] = revad
	}
	return revads, nil
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
