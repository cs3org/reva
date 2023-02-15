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

package grpc_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cs3org/reva/tests/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

const timeoutMs = 30000

var mutex = sync.Mutex{}
var port = 25000

func TestGrpc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Grpc Suite")
}

type cleanupFunc func(bool) error

// Revad represents a running revad process.
type Revad struct {
	TmpRoot     string      // Temporary directory on disk. Will be cleaned up by the Cleanup func.
	GrpcAddress string      // Address of the grpc service
	Cleanup     cleanupFunc // Function to kill the process and cleanup the temp. root. If the given parameter is true the files will be kept to make debugging failures easier.
}

type res struct{}

func (res) isResource() {}

type Resource interface {
	isResource()
}

type Folder struct {
	res
}

type File struct {
	res
	Content  any
	Encoding string // json, plain
}

// stardRevads takes a list of revad configuration files plus a map of
// variables that need to be substituted in them and starts them.
//
// A unique port is assigned to each spawned instance.
// Placeholders in the config files can be replaced the variables from the
// `variables` map, e.g. the config
//
//	redis = "{{redis_address}}"
//
// and the variables map
//
//	variables = map[string]string{"redis_address": "localhost:6379"}
//
// will result in the config
//
//	redis = "localhost:6379"
//
// Special variables are created for the revad addresses, e.g. having a
// `storage` and a `users` revad will make `storage_address` and
// `users_address` available wit the dynamically assigned ports so that
// the services can be made available to each other.
func startRevads(configs map[string]string, externalFiles map[string]string, newResources map[string]Resource, variables map[string]string) (map[string]*Revad, error) {
	mutex.Lock()
	defer mutex.Unlock()

	tmpRoot, err := helpers.TempDir("")
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create tmpdir for")
	}

	var revads sync.Map //  map[string]*Revad{}
	addresses := map[string]string{}
	filesPath := map[string]string{}
	for name := range configs {
		addresses[name] = fmt.Sprintf("localhost:%d", port)
		port++
	}
	for name, p := range externalFiles {
		rawFile, err := os.ReadFile(path.Join("fixtures", p))
		if err != nil {
			return nil, errors.Wrapf(err, "error reading file")
		}
		cfg := string(rawFile)
		for v, value := range variables {
			cfg = strings.ReplaceAll(cfg, "{{"+v+"}}", value)
		}
		for name, address := range addresses {
			cfg = strings.ReplaceAll(cfg, "{{"+name+"_address}}", address)
		}
		newFilePath := path.Join(tmpRoot, p)
		err = os.WriteFile(newFilePath, []byte(cfg), 0600)
		if err != nil {
			return nil, errors.Wrapf(err, "error writing file")
		}
		filesPath[name] = newFilePath
	}
	for name, resource := range newResources {
		tmpResourcePath := filepath.Join(tmpRoot, name)

		switch r := resource.(type) {
		case File:
			// fill the file with the initial content
			switch r.Encoding {
			case "", "plain":
				if err := os.WriteFile(tmpResourcePath, []byte(r.Content.(string)), 0644); err != nil {
					return nil, err
				}
			case "json":
				d, err := json.Marshal(r.Content)
				if err != nil {
					return nil, err
				}
				if err := os.WriteFile(tmpResourcePath, d, 0644); err != nil {
					return nil, err
				}
			default:
				return nil, errors.New("encoding not known " + r.Encoding)
			}
		case Folder:
			if err := os.MkdirAll(tmpResourcePath, 0755); err != nil {
				return nil, err
			}
		}

		filesPath[name] = tmpResourcePath
	}

	g := new(errgroup.Group)

	for name, config := range configs {
		name, config := name, config
		g.Go(func() error {
			ownAddress := addresses[name]

			newCfgPath := path.Join(tmpRoot, config)
			rawCfg, err := os.ReadFile(path.Join("fixtures", config))
			if err != nil {
				return errors.Wrapf(err, "Could not read config file")
			}
			cfg := string(rawCfg)
			cfg = strings.ReplaceAll(cfg, "{{root}}", tmpRoot)
			for name, path := range filesPath {
				cfg = strings.ReplaceAll(cfg, "{{file_"+name+"}}", path)
				cfg = strings.ReplaceAll(cfg, "{{"+name+"}}", path)
			}
			cfg = strings.ReplaceAll(cfg, "{{grpc_address}}", ownAddress)
			if url, ok := addresses["gateway"]; ok {
				cfg = strings.ReplaceAll(cfg, "{{gateway_address}}", url)
			}
			for v, value := range variables {
				cfg = strings.ReplaceAll(cfg, "{{"+v+"}}", value)
			}
			for name, address := range addresses {
				cfg = strings.ReplaceAll(cfg, "{{"+name+"_address}}", address)
			}
			if err := os.MkdirAll(filepath.Dir(newCfgPath), 0755); err != nil {
				return errors.Wrapf(err, "cannot create config folders")
			}
			err = os.WriteFile(newCfgPath, []byte(cfg), 0644)
			if err != nil {
				return errors.Wrapf(err, "Could not write config file")
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
				return errors.Wrapf(err, "Could not start revad")
			}

			err = waitForPort(ownAddress, "open")
			if err != nil {
				return err
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
			revads.Store(name, revad)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	res := make(map[string]*Revad)
	revads.Range(func(key, value any) bool {
		res[key.(string)] = value.(*Revad)
		return true
	})

	return res, nil
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
