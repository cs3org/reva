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

package json

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"bou.ke/monkey"
	apppb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/gdexlab/go-render/render"
	"github.com/sethvargo/go-password/password"
	"golang.org/x/crypto/bcrypt"
)

func TestNewManager(t *testing.T) {
	userTest := &userpb.User{Id: &userpb.UserId{Idp: "0"}, Username: "Test User"}

	// temp directory where are stored tests config files
	tempDir := createTempDir(t, "jsonappauth_test")
	defer os.RemoveAll(tempDir)

	jsonCorruptedFile := createTempFile(t, tempDir, "corrupted.json")
	defer jsonCorruptedFile.Close()
	jsonEmptyFile := createTempFile(t, tempDir, "empty.json")
	defer jsonEmptyFile.Close()
	jsonOkFile := createTempFile(t, tempDir, "ok.json")
	defer jsonOkFile.Close()

	hashToken, _ := bcrypt.GenerateFromPassword([]byte("1234"), 10)

	dummyData := map[string]map[string]*apppb.AppPassword{
		userTest.GetId().String(): {
			string(hashToken): {
				Password:   string(hashToken),
				TokenScope: nil,
				Label:      "label",
				User:       userTest.GetId(),
				Expiration: nil,
				Ctime:      &typespb.Timestamp{Seconds: 0},
				Utime:      &typespb.Timestamp{Seconds: 0},
			},
		}}

	dummyDataJSON, _ := json.Marshal(dummyData)

	// fill temp file with tests data
	fill(t, jsonCorruptedFile, `[{`)
	fill(t, jsonEmptyFile, "")
	fill(t, jsonOkFile, string(dummyDataJSON))

	testCases := []struct {
		description string
		configMap   map[string]interface{}
		expected    *jsonManager
	}{
		{
			description: "New appauth manager from corrupted state file",
			configMap: map[string]interface{}{
				"file":           jsonCorruptedFile.Name(),
				"token_strength": 10,
			},
			expected: nil, // nil == error
		},
		{
			description: "New appauth manager from empty state file",
			configMap: map[string]interface{}{
				"file":               jsonEmptyFile.Name(),
				"token_strength":     10,
				"password_hash_cost": 12,
			},
			expected: &jsonManager{
				config: &config{
					File:             jsonEmptyFile.Name(),
					TokenStrength:    10,
					PasswordHashCost: 12,
				},
				passwords: map[string]map[string]*apppb.AppPassword{},
			},
		},
		{
			description: "New appauth manager from state file",
			configMap: map[string]interface{}{
				"file":               jsonOkFile.Name(),
				"token_strength":     10,
				"password_hash_cost": 10,
			},
			expected: &jsonManager{
				config: &config{
					File:             jsonOkFile.Name(),
					TokenStrength:    10,
					PasswordHashCost: 10,
				},
				passwords: dummyData,
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			manager, err := New(test.configMap)
			if test.expected == nil {
				if err == nil {
					t.Fatalf("no error (but we expected one) while get manager")
				} else {
					t.Skip()
				}
			}
			if !reflect.DeepEqual(test.expected, manager) {
				t.Fatalf("appauth differ: expected=%v got=%v", render.AsCode(test.expected), render.AsCode(manager))
			}
		})
	}

}

func TestGenerateAppPassword(t *testing.T) {
	userTest := &userpb.User{Id: &userpb.UserId{Idp: "0"}, Username: "Test User"}
	ctx := ctxpkg.ContextSetUser(context.Background(), userTest)
	tempDir := createTempDir(t, "jsonappauth_test")
	defer os.RemoveAll(tempDir)

	nowFixed := time.Date(2021, time.May, 21, 12, 21, 0, 0, time.UTC)
	patchNow := monkey.Patch(time.Now, func() time.Time { return nowFixed })
	now := now()
	token := "1234"
	patchPasswordGenerate := monkey.Patch(password.Generate, func(int, int, int, bool, bool) (string, error) { return token, nil })
	defer patchNow.Unpatch()
	defer patchPasswordGenerate.Unpatch()

	generateFromPassword := monkey.Patch(bcrypt.GenerateFromPassword, func(pw []byte, n int) ([]byte, error) {
		return append([]byte("hash:"), pw...), nil
	})
	defer generateFromPassword.Restore()
	hashTokenXXXX, _ := bcrypt.GenerateFromPassword([]byte("XXXX"), 11)
	hashToken1234, _ := bcrypt.GenerateFromPassword([]byte(token), 11)

	dummyData := map[string]map[string]*apppb.AppPassword{
		userpb.User{Id: &userpb.UserId{Idp: "1"}, Username: "Test User1"}.Id.String(): {
			string(hashTokenXXXX): {
				Password: string(hashTokenXXXX),
				Label:    "",
				User:     &userpb.UserId{Idp: "1"},
				Ctime:    now,
				Utime:    now,
			},
		},
	}

	dummyDataJSON, _ := json.Marshal(dummyData)

	testCases := []struct {
		description   string
		prevStateJSON string
		expected      *apppb.AppPassword
		expectedState map[string]map[string]*apppb.AppPassword
	}{
		{
			description:   "GenerateAppPassword with empty state",
			prevStateJSON: `{}`,
			expected: &apppb.AppPassword{
				Password:   token,
				TokenScope: nil,
				Label:      "label",
				User:       userTest.GetId(),
				Expiration: nil,
				Ctime:      now,
				Utime:      now,
			},
			expectedState: map[string]map[string]*apppb.AppPassword{
				userTest.GetId().String(): {
					string(hashToken1234): {
						Password:   string(hashToken1234),
						TokenScope: nil,
						Label:      "label",
						User:       userTest.GetId(),
						Expiration: nil,
						Ctime:      now,
						Utime:      now,
					},
				},
			},
		},
		{
			description:   "GenerateAppPassword with not empty state",
			prevStateJSON: string(dummyDataJSON),
			expected: &apppb.AppPassword{
				Password:   token,
				TokenScope: nil,
				Label:      "label",
				User:       userTest.GetId(),
				Expiration: nil,
				Ctime:      now,
				Utime:      now,
			},
			expectedState: concatMaps(map[string]map[string]*apppb.AppPassword{
				userTest.GetId().String(): {
					string(hashToken1234): {
						Password:   string(hashToken1234),
						TokenScope: nil,
						Label:      "label",
						User:       userTest.GetId(),
						Expiration: nil,
						Ctime:      now,
						Utime:      now,
					},
				}},
				dummyData),
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			// initialize temp file with `prevStateJSON` content
			tmpFile := createTempFile(t, tempDir, "test.json")
			defer tmpFile.Close()
			fill(t, tmpFile, test.prevStateJSON)
			manager, err := New(map[string]interface{}{
				"file":               tmpFile.Name(),
				"token_strength":     len(token),
				"password_hash_cost": 11,
			})
			if err != nil {
				t.Fatal("error creating manager:", err)
			}

			pw, err := manager.GenerateAppPassword(ctx, nil, "label", nil)
			if err != nil {
				t.Fatal("error generating password:", err)
			}

			// test state in memory

			if !reflect.DeepEqual(pw, test.expected) {
				t.Fatalf("apppassword differ: expected=%v got=%v", render.AsCode(test.expected), render.AsCode(pw))
			}

			if !reflect.DeepEqual(manager.(*jsonManager).passwords, test.expectedState) {
				t.Fatalf("manager state differ: expected=%v got=%v", render.AsCode(test.expectedState), render.AsCode(manager.(*jsonManager).passwords))
			}

			// test saved json

			_, err = tmpFile.Seek(0, 0)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(tmpFile)
			if err != nil {
				t.Fatalf("error reading file %s: %v", tmpFile.Name(), err)
			}

			var jsonState map[string]map[string]*apppb.AppPassword
			err = json.Unmarshal(data, &jsonState)
			if err != nil {
				t.Fatalf("error decoding json: %v", err)
			}

			if !reflect.DeepEqual(jsonState, test.expectedState) {
				t.Fatalf("json state differ: expected=%v got=%v", render.AsCode(jsonState), render.AsCode(test.expectedState))
			}

		})
	}

}

func TestListAppPasswords(t *testing.T) {
	user0Test := &userpb.User{Id: &userpb.UserId{Idp: "0"}}
	user1Test := &userpb.User{Id: &userpb.UserId{Idp: "1"}}
	ctx := ctxpkg.ContextSetUser(context.Background(), user0Test)
	tempDir := createTempDir(t, "jsonappauth_test")
	defer os.RemoveAll(tempDir)

	nowFixed := time.Date(2021, time.May, 21, 12, 21, 0, 0, time.UTC)
	patchNow := monkey.Patch(time.Now, func() time.Time { return nowFixed })
	defer patchNow.Unpatch()
	now := now()

	token := "hash:1234"

	dummyDataUser0 := map[string]map[string]*apppb.AppPassword{
		user0Test.GetId().String(): {
			token: {
				Password:   token,
				TokenScope: nil,
				Label:      "label",
				User:       user0Test.GetId(),
				Expiration: nil,
				Ctime:      now,
				Utime:      now,
			},
		}}

	dummyDataUserExpired := map[string]map[string]*apppb.AppPassword{
		user0Test.GetId().String(): {
			token: {
				Password:   token,
				TokenScope: nil,
				Label:      "label",
				User:       user0Test.GetId(),
				Expiration: &typespb.Timestamp{
					Seconds: 100,
				},
				Ctime: now,
				Utime: now,
			},
		}}

	dummyDataUser0JSON, _ := json.Marshal(dummyDataUser0)
	dummyDataUserExpiredJSON, _ := json.Marshal(dummyDataUserExpired)

	dummyDataUser1 := map[string]map[string]*apppb.AppPassword{
		user1Test.GetId().String(): {
			"XXXX": {
				Password:   "XXXX",
				TokenScope: nil,
				Label:      "label",
				User:       user1Test.GetId(),
				Expiration: nil,
				Ctime:      now,
				Utime:      now,
			},
		}}

	dummyDataTwoUsersJSON, _ := json.Marshal(concatMaps(dummyDataUser0, dummyDataUser1))

	testCases := []struct {
		description   string
		stateJSON     string
		expectedState []*apppb.AppPassword
	}{
		{
			description:   "ListAppPasswords with empty state",
			stateJSON:     `{}`,
			expectedState: make([]*apppb.AppPassword, 0),
		},
		{
			description:   "ListAppPasswords with not json state file",
			stateJSON:     "",
			expectedState: make([]*apppb.AppPassword, 0),
		},
		{
			description: "ListAppPasswords with not empty state (only one user)",
			stateJSON:   string(dummyDataUser0JSON),
			expectedState: []*apppb.AppPassword{
				dummyDataUser0[user0Test.GetId().String()][token],
			},
		},
		{
			description: "ListAppPasswords with not empty state with expired password (only one user)",
			stateJSON:   string(dummyDataUserExpiredJSON),
			expectedState: []*apppb.AppPassword{
				dummyDataUserExpired[user0Test.GetId().String()][token],
			},
		},
		{
			description: "ListAppPasswords with not empty state (different users)",
			stateJSON:   string(dummyDataTwoUsersJSON),
			expectedState: []*apppb.AppPassword{
				dummyDataUser0[user0Test.GetId().String()][token],
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			// initialize temp file with `state_json` content
			tmpFile := createTempFile(t, tempDir, "test.json")
			defer tmpFile.Close()
			if test.stateJSON != "" {
				fill(t, tmpFile, test.stateJSON)
			}
			manager, err := New(map[string]interface{}{
				"file":           tmpFile.Name(),
				"token_strength": len(token),
			})
			if err != nil {
				t.Fatal("error creating manager:", err)
			}

			pws, err := manager.ListAppPasswords(ctx)
			if err != nil {
				t.Fatal("error listing passwords:", err)
			}

			if !reflect.DeepEqual(pws, test.expectedState) {
				t.Fatalf("list passwords differ: expected=%v got=%v", test.expectedState, pws)
			}

		})
	}

}

func TestInvalidateAppPassword(t *testing.T) {
	userTest := &userpb.User{Id: &userpb.UserId{Idp: "0"}}
	ctx := ctxpkg.ContextSetUser(context.Background(), userTest)
	tempDir := createTempDir(t, "jsonappauth_test")
	defer os.RemoveAll(tempDir)

	nowFixed := time.Date(2021, time.May, 21, 12, 21, 0, 0, time.UTC)
	patchNow := monkey.Patch(time.Now, func() time.Time { return nowFixed })
	now := now()
	defer patchNow.Unpatch()

	token := "hash:1234"

	dummyDataUser1Token := map[string]map[string]*apppb.AppPassword{
		userTest.GetId().String(): {
			token: {
				Password:   token,
				TokenScope: nil,
				Label:      "label",
				User:       userTest.GetId(),
				Expiration: nil,
				Ctime:      now,
				Utime:      now,
			},
		}}

	dummyDataUser1TokenJSON, _ := json.Marshal(dummyDataUser1Token)

	dummyDataUser2Token := map[string]map[string]*apppb.AppPassword{
		userTest.GetId().String(): {
			token: {
				Password:   token,
				TokenScope: nil,
				Label:      "label",
				User:       userTest.GetId(),
				Expiration: nil,
				Ctime:      now,
				Utime:      now,
			},
			"hash:XXXX": {
				Password:   "hash:XXXX",
				TokenScope: nil,
				Label:      "label",
				User:       userTest.GetId(),
				Expiration: nil,
				Ctime:      now,
				Utime:      now,
			},
		}}

	dummyDataUser2TokenJSON, _ := json.Marshal(dummyDataUser2Token)

	testCases := []struct {
		description   string
		stateJSON     string
		password      string
		expectedState map[string]map[string]*apppb.AppPassword
	}{
		{
			description:   "InvalidateAppPassword with empty state",
			stateJSON:     `{}`,
			password:      "TOKEN_NOT_EXISTS",
			expectedState: nil,
		},
		{
			description:   "InvalidateAppPassword with not empty state and token does not exist",
			stateJSON:     string(dummyDataUser1TokenJSON),
			password:      "TOKEN_NOT_EXISTS",
			expectedState: nil,
		},
		{
			description:   "InvalidateAppPassword with not empty state and token exists",
			stateJSON:     string(dummyDataUser1TokenJSON),
			password:      token,
			expectedState: map[string]map[string]*apppb.AppPassword{},
		},
		{
			description: "InvalidateAppPassword with user that has more than 1 token",
			stateJSON:   string(dummyDataUser2TokenJSON),
			password:    token,
			expectedState: map[string]map[string]*apppb.AppPassword{
				userTest.GetId().String(): {
					"hash:XXXX": {
						Password:   "hash:XXXX",
						TokenScope: nil,
						Label:      "label",
						User:       userTest.GetId(),
						Expiration: nil,
						Ctime:      now,
						Utime:      now,
					},
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			// initialize temp file with `state_json` content
			tmpFile := createTempFile(t, tempDir, "test.json")
			fill(t, tmpFile, test.stateJSON)
			manager, err := New(map[string]interface{}{
				"file":           tmpFile.Name(),
				"token_strength": 4,
			})
			if err != nil {
				t.Fatal("error creating manager:", err)
			}

			err = manager.InvalidateAppPassword(ctx, test.password)
			if test.expectedState == nil {
				if err == nil {
					t.Fatalf("no error (but we expected one) while get manager")
				} else {
					t.Skip()
				}
			}
			if !reflect.DeepEqual(test.expectedState, manager.(*jsonManager).passwords) {
				t.Fatalf("apppauth state differ: expected=%v got=%v", render.AsCode(test.expectedState), render.AsCode(manager.(*jsonManager).passwords))
			}

		})
	}

}

func TestGetAppPassword(t *testing.T) {
	userTest := &userpb.User{Id: &userpb.UserId{Idp: "0"}}
	ctx := ctxpkg.ContextSetUser(context.Background(), userTest)
	tempDir := createTempDir(t, "jsonappauth_test")
	defer os.RemoveAll(tempDir)

	nowFixed := time.Date(2021, time.May, 21, 12, 21, 0, 0, time.UTC)
	patchNow := monkey.Patch(time.Now, func() time.Time { return nowFixed })
	defer patchNow.Unpatch()

	now := now()
	token := "1234"

	generateFromPassword := monkey.Patch(bcrypt.GenerateFromPassword, func(pw []byte, n int) ([]byte, error) {
		return append([]byte("hash:"), pw...), nil
	})
	compareHashAndPassword := monkey.Patch(bcrypt.CompareHashAndPassword, func(hash, pw []byte) error {
		hashPw, _ := bcrypt.GenerateFromPassword(pw, 0)
		if bytes.Equal(hashPw, hash) {
			return nil
		}
		return bcrypt.ErrMismatchedHashAndPassword
	})
	defer generateFromPassword.Restore()
	defer compareHashAndPassword.Restore()
	hashToken1234, _ := bcrypt.GenerateFromPassword([]byte(token), 11)

	dummyDataUser1Token := map[string]map[string]*apppb.AppPassword{
		userTest.GetId().String(): {
			string(hashToken1234): {
				Password:   string(hashToken1234),
				TokenScope: nil,
				Label:      "label",
				User:       userTest.GetId(),
				Expiration: nil,
				Ctime:      now,
				Utime:      now,
			},
		}}

	dummyDataUserExpired := map[string]map[string]*apppb.AppPassword{
		userTest.GetId().String(): {
			string(hashToken1234): {
				Password:   string(hashToken1234),
				TokenScope: nil,
				Label:      "label",
				User:       userTest.GetId(),
				Expiration: &typespb.Timestamp{
					Seconds: 100,
				},
				Ctime: now,
				Utime: now,
			},
		}}

	dummyDataUserFutureExpiration := map[string]map[string]*apppb.AppPassword{
		userTest.GetId().String(): {
			string(hashToken1234): {
				Password:   string(hashToken1234),
				TokenScope: nil,
				Label:      "label",
				User:       userTest.GetId(),
				Expiration: &typespb.Timestamp{
					Seconds: uint64(time.Now().Unix()) + 3600,
				},
				Ctime: now,
				Utime: now,
			},
		}}

	dummyDataUser1TokenJSON, _ := json.Marshal(dummyDataUser1Token)
	dummyDataUserExpiredJSON, _ := json.Marshal(dummyDataUserExpired)
	dummyDataUserFutureExpirationJSON, _ := json.Marshal(dummyDataUserFutureExpiration)

	dummyDataDifferentUserToken := map[string]map[string]*apppb.AppPassword{
		"OTHER_USER_ID": {
			string(hashToken1234): {
				Password:   string(hashToken1234),
				TokenScope: nil,
				Label:      "label",
				User:       &userpb.UserId{Idp: "OTHER_USER_ID"},
				Expiration: nil,
				Ctime:      now,
				Utime:      now,
			},
		}}

	dummyDataDifferentUserTokenJSON, _ := json.Marshal(dummyDataDifferentUserToken)

	testCases := []struct {
		description   string
		stateJSON     string
		password      string
		expectedState *apppb.AppPassword
	}{
		{
			description:   "GetAppPassword with token that does not exist",
			stateJSON:     string(dummyDataUser1TokenJSON),
			password:      "TOKEN_NOT_EXISTS",
			expectedState: nil,
		},
		{
			description:   "GetAppPassword with expired token",
			stateJSON:     string(dummyDataUserExpiredJSON),
			password:      "1234",
			expectedState: nil,
		},
		{
			description:   "GetAppPassword with token with expiration set in the future",
			stateJSON:     string(dummyDataUserFutureExpirationJSON),
			password:      "1234",
			expectedState: dummyDataUserFutureExpiration[userTest.GetId().String()][string(hashToken1234)],
		},
		{
			description:   "GetAppPassword with token that exists but different user",
			stateJSON:     string(dummyDataDifferentUserTokenJSON),
			password:      "1234",
			expectedState: nil,
		},
		{
			description:   "GetAppPassword with token that exists owned by user",
			stateJSON:     string(dummyDataUser1TokenJSON),
			password:      "1234",
			expectedState: dummyDataUser1Token[userTest.GetId().String()][string(hashToken1234)],
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			// initialize temp file with `state_json` content
			tmpFile := createTempFile(t, tempDir, "test.json")
			fill(t, tmpFile, test.stateJSON)
			manager, err := New(map[string]interface{}{
				"file":           tmpFile.Name(),
				"token_strength": 4,
			})
			if err != nil {
				t.Fatal("error creating manager:", err)
			}

			pw, err := manager.GetAppPassword(ctx, userTest.GetId(), test.password)
			if test.expectedState == nil {
				if err == nil {
					t.Fatalf("no error (but we expected one) while get manager")
				} else {
					t.Skip()
				}
			}
			if !reflect.DeepEqual(test.expectedState, pw) {
				t.Fatalf("apppauth state differ: expected=%v got=%v", render.AsCode(test.expectedState), render.AsCode(pw))
			}

		})
	}
}

func createTempDir(t *testing.T, name string) string {
	tempDir, err := os.MkdirTemp("", name)
	if err != nil {
		t.Fatalf("error while creating temp dir: %v", err)
	}
	return tempDir
}

func createTempFile(t *testing.T, tempDir string, name string) *os.File {
	tempFile, err := os.CreateTemp(tempDir, name)
	if err != nil {
		t.Fatalf("error while creating temp file: %v", err)
	}
	return tempFile
}

func fill(t *testing.T, file *os.File, data string) {
	_, err := file.WriteString(data)
	if err != nil {
		t.Fatalf("error while writing to file: %v", err)
	}
}

func concatMaps(maps ...map[string]map[string]*apppb.AppPassword) map[string]map[string]*apppb.AppPassword {
	res := make(map[string]map[string]*apppb.AppPassword)
	for _, m := range maps {
		for k := range m {
			res[k] = m[k]
		}
	}
	return res
}
