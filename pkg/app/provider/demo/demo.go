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

package demo

import (
	"context"
	"fmt"

	"github.com/cs3org/reva/pkg/app"

	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/mitchellh/mapstructure"
)

type provider struct {
	iframeUIProvider string
}

func (p *provider) GetIFrame(ctx context.Context, resID *providerpb.ResourceId, token string) (string, error) {
	msg := fmt.Sprintf("<iframe src=%s/open/%s?access-token=%s />", p.iframeUIProvider, resID.StorageId+":"+resID.OpaqueId, token)
	return msg, nil
}

type config struct {
	IFrameUIProvider string `mapstructure:"iframe_ui_provider"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (app.Provider, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &provider{iframeUIProvider: c.IFrameUIProvider}, nil
}
