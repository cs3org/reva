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

package memory

import (
	"context"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/share"
	"reflect"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	type args struct {
		c map[string]interface{}
	}

	var instance = new(mgr)

	tests := []struct {
		name    string
		args    args
		want    share.Manager
		wantErr bool
	}{
		{
			name: "create new instance",
			args: args{
				c: nil,
			},
			want:    instance,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("New() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_mgr_GetShare(t *testing.T) {
	type fields struct {
		shares sync.Map
	}
	type args struct {
		ctx context.Context
		ref *ocm.ShareReference
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *ocm.Share
		wantErr bool
	}{
		{
			name: "Getting not exist share",
			fields: fields{
				shares: sync.Map{},
			},
			args: args{
				ctx: nil,
				ref: &ocm.ShareReference{
					Spec: &ocm.ShareReference_Id{
						Id: &ocm.ShareId{
							OpaqueId: "1234",
						},
					},
				},
			},
			// want: &ocm.Share{
			// 	Id: &ocm.ShareId{
			// 		OpaqueId: "1234",
			// 	},
			// },
			want: nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mgr{
				shares: tt.fields.shares,
			}

			got, err := m.GetShare(tt.args.ctx, tt.args.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetShare() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetShare() got = %v, want %v", got, tt.want)
			}
		})
	}
}
