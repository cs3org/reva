// Copyright 2018-2020 CERN
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

package action_test

import (
	"fmt"
	"testing"

	"github.com/cs3org/reva/pkg/sdk/action"
	testintl "github.com/cs3org/reva/pkg/sdk/common/testing"
)

func TestActions(t *testing.T) {
	tests := []struct {
		host     string
		username string
		password string
	}{
		{"sciencemesh-test.uni-muenster.de:9600", "test", "testpass"},
	}

	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			// Prepare the session
			if session, err := testintl.CreateTestSession(test.host, test.username, test.password); err == nil {
				// Try creating a directory
				if act, err := action.NewFileOperationsAction(session); err == nil {
					if err := act.MakePath("/home/subdir/subsub"); err != nil {
						t.Errorf(testintl.FormatTestError("FileOperationsAction.MakePath", err, "/home/subdir/subsub"))
					}
				} else {
					t.Errorf(testintl.FormatTestError("NewFileOperationsAction", err, session))
				}

				// Try uploading
				if act, err := action.NewUploadAction(session); err == nil {
					act.EnableTUS = true
					if _, err := act.UploadBytes([]byte("HELLO WORLD!\n"), "/home/subdir/tests.txt"); err != nil {
						t.Errorf(testintl.FormatTestError("UploadAction.UploadBytes", err, []byte("HELLO WORLD!\n"), "/home/subdir/tests.txt"))
					}
				} else {
					t.Errorf(testintl.FormatTestError("NewUploadAction", err, session))
				}

				// Try moving
				if act, err := action.NewFileOperationsAction(session); err == nil {
					if err := act.MoveTo("/home/subdir/tests.txt", "/home/subdir/subtest"); err != nil {
						t.Errorf(testintl.FormatTestError("FileOperationsAction.MoveTo", err, "/home/subdir/tests.txt", "/home/subdir/subtest"))
					}
				} else {
					t.Errorf(testintl.FormatTestError("NewFileOperationsAction", err, session))
				}

				// Try downloading
				if act, err := action.NewDownloadAction(session); err == nil {
					if _, err := act.DownloadFile("/home/subdir/subtest/tests.txt"); err != nil {
						t.Errorf(testintl.FormatTestError("DownloadAction.DownloadFile", err, "/home/subdir/subtest/tests.txt"))
					}
				} else {
					t.Errorf(testintl.FormatTestError("NewDownloadAction", err, session))
				}

				// Try listing
				if act, err := action.NewEnumFilesAction(session); err == nil {
					if _, err := act.ListFiles("/home", true); err != nil {
						t.Errorf(testintl.FormatTestError("EnumFilesAction.ListFiles", err, "/home", true))
					}
				} else {
					t.Errorf(testintl.FormatTestError("NewEnumFilesAction", err, session))
				}

				// Try deleting a directory
				if act, err := action.NewFileOperationsAction(session); err == nil {
					if err := act.Remove("/home/subdir"); err != nil {
						t.Errorf(testintl.FormatTestError("FileOperationsAction.Remove", err, "/home/subdir"))
					}
				} else {
					t.Errorf(testintl.FormatTestError("NewFileOperationsAction", err, session))
				}

				// Try accessing some files and directories
				if act, err := action.NewFileOperationsAction(session); err == nil {
					if act.FileExists("/home/blargh.txt") {
						t.Errorf(testintl.FormatTestError("FileOperationsAction.FileExists", fmt.Errorf("non-existing file reported as existing"), "/home/blargh.txt"))
					}

					if !act.DirExists("/home") {
						t.Errorf(testintl.FormatTestError("FileOperationsAction.DirExists", fmt.Errorf("/home dir reported as non-existing"), "/home"))
					}
				} else {
					t.Errorf(testintl.FormatTestError("NewFileOperationsAction", err, session))
				}
			} else {
				t.Errorf(testintl.FormatTestError("CreateTestSession", err))
			}
		})
	}
}
