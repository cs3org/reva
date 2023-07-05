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

package registry

import (
	"context"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/notification/handler"
)

// NewHandlerFunc is the function that notification handlers should register to
// at init time.
type NewHandlerFunc func(context.Context, map[string]any) (handler.Handler, error)

// NewHandlerFuncs is a map containing all the registered notification handlers.
var NewHandlerFuncs = map[string]NewHandlerFunc{}

// Register registers a new notification handler new function. Not safe for
// concurrent use. Safe for use from package init.
func Register(name string, f NewHandlerFunc) {
	NewHandlerFuncs[name] = f
}

// InitHandlers initializes the notification handlers with the configuration
// and the log from a service.
func InitHandlers(ctx context.Context, handlerConf map[string]map[string]any) map[string]handler.Handler {
	handlers := make(map[string]handler.Handler)
	hCount := 0

	log := appctx.GetLogger(ctx)
	for n, f := range NewHandlerFuncs {
		if c, ok := handlerConf[n]; ok {
			l := log.With().Str("service", n).Logger()
			ctx := appctx.WithLogger(ctx, &l)
			nh, err := f(ctx, c)
			if err != nil {
				log.Err(err).Msgf("error initializing notification handler %s", n)
			}
			handlers[n] = nh
			hCount++
		} else {
			log.Warn().Msgf("missing config for notification handler %s", n)
		}
	}
	log.Info().Msgf("%d handlers initialized", hCount)

	return handlers
}
