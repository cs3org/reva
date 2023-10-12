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

package main

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

var testCommand = func() *command {
	cmd := newCommand("teststorageperf")
	cmd.Description = func() string {
		return "Little performance test: upload/download/remove 1000 files into the directory /home/perftest. The source file is /tmp/1MFile"
	}
	cmd.Action = func(w ...io.Writer) error {

		start := time.Now()

		b, err := executeCommand(mkdirCommand(), "/home/testperf")

		if err != nil {
			fmt.Println("Error doing mkdir: ", b, err)
			return nil
		}

		elapsedmkdir := time.Since(start)

		start = time.Now()

		for i := 0; i < 1000; i++ {

			b, err := executeCommand(uploadCommand(), "-protocol", "simple", "/tmp/1MFile", "/home/testperf/file-"+strconv.FormatInt(int64(i), 10))

			if err != nil {
				fmt.Printf("Error uploading file %d\n", i)
				fmt.Println("Err:", b, err)
				return nil
			}
		}

		elapsedupload := time.Since(start)

		start = time.Now()

		for i := 0; i < 1000; i++ {

			b, err := executeCommand(downloadCommand(), "/home/testperf/file-"+strconv.FormatInt(int64(i), 10), "/tmp/1Mdeleteme")

			if err != nil {
				fmt.Printf("Error downloading file %d\n", i)
				fmt.Println("Err:", b, err)
				return nil
			}
		}

		elapseddownload := time.Since(start)

		start = time.Now()

		for i := 0; i < 1000; i++ {

			b, err := executeCommand(rmCommand(), "/home/testperf/file-"+strconv.FormatInt(int64(i), 10))

			if err != nil {
				fmt.Printf("Error removing file %d\n", i)
				fmt.Println("Err:", b, err)
				return nil
			}
		}

		elapsedrm := time.Since(start)

		fmt.Printf("mkdir took %s \n", elapsedmkdir)
		fmt.Printf("upload took %s \n", elapsedupload)
		fmt.Printf("download took %s \n", elapseddownload)
		fmt.Printf("rm took %s \n", elapsedrm)

		return nil
	}
	return cmd
}
