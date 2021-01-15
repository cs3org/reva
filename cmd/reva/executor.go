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

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"
)

// Executor provides exec command handler
type Executor struct {
	Timeout int
}

// Execute provides execute commands
func (e *Executor) Execute(s string) {
	s = strings.TrimSpace(s)
	switch s {
	case "":
		return
	case "exit", "quit":
		os.Exit(0)
	}

	args := strings.Split(s, " ")

	// Verify that the configuration is set, either in memory or in a file.
	if conf == nil || conf.Host == "" {
		c, err := readConfig()
		if err != nil && args[0] != "configure" {
			fmt.Println("reva is not configured, please pass the -host flag or run the configure command")
			return
		} else if args[0] != "configure" {
			conf = c
		}
	}

	action := args[0]
	for _, v := range commands {
		if v.Name == action {
			if err := v.Parse(args[1:]); err != nil {
				fmt.Println(err)
				return
			}
			defer v.ResetFlags()

			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			signalChan := make(chan os.Signal, 1)
			signal.Notify(signalChan, os.Interrupt)
			defer func() {
				signal.Stop(signalChan)
				cancel()
			}()

			go func() {
				if e.Timeout > 0 {
					select {
					case <-signalChan:
						cancel()
					case <-time.After(time.Duration(e.Timeout * int(time.Second))):
						cancel()
					case <-ctx.Done():
					}
				} else {
					select {
					case <-signalChan:
						cancel()
					case <-ctx.Done():
					}
				}
			}()

			if err := executeWithContext(ctx, v); err != nil {
				fmt.Println(err.Error())
			}
			return
		}
	}

	fmt.Println("Invalid command. Use \"help\" to list the available commands.")
}

func executeWithContext(ctx context.Context, cmd *command) error {
	c := make(chan error, 1)
	go func() {
		c <- cmd.Action()
	}()
	select {
	case <-ctx.Done():
		return errors.New("Cancelled by user")
	case err := <-c:
		return err
	}
}
