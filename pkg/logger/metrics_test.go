// Copyright 2018-2026 CERN
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

package logger

import (
	"io"
	"testing"

	"github.com/rs/zerolog"
)

func counts() (warn, errs float64) {
	return float64(logMessages[zerolog.WarnLevel].Load()), float64(logMessages[zerolog.ErrorLevel].Load())
}

func infoCounted() bool {
	_, ok := logMessages[zerolog.InfoLevel]
	return ok
}

func TestLogMessagesCounter(t *testing.T) {
	l := New(WithWriter(io.Discard, JSONMode))
	w0, e0 := counts()
	l.Error().Msg("e")
	l.Warn().Msg("w")
	l.Warn().Msg("w")
	l.Info().Msg("i")
	l.Debug().Msg("d")
	w1, e1 := counts()
	if w1-w0 != 2 || e1-e0 != 1 {
		t.Fatalf("got warn +%v error +%v, want +2 +1", w1-w0, e1-e0)
	}
	if infoCounted() {
		t.Fatal("info must not be counted")
	}
}

func TestLogMessagesCounterHonoursLevel(t *testing.T) {
	defer zerolog.SetGlobalLevel(zerolog.GlobalLevel())
	for name, opt := range map[string]Option{"WithLevel": WithLevel("error"), "WithRuntimeLevel": WithRuntimeLevel("error")} {
		l := New(WithWriter(io.Discard, JSONMode), opt)
		w0, _ := counts()
		l.Warn().Msg("w")
		if w1, _ := counts(); w1 != w0 {
			t.Errorf("%s: filtered warn was counted", name)
		}
	}
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	l := New(WithWriter(io.Discard, JSONMode), WithRuntimeLevel("info"))
	SetLevel("error")
	w0, _ := counts()
	l.Warn().Msg("w")
	if w1, _ := counts(); w1 != w0 {
		t.Error("warn filtered by SetLevel was counted")
	}
}
