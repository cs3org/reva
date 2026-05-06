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

package labelsprovider

import (
	"context"

	labelsv1beta1 "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/labels"
	"github.com/cs3org/reva/v3/pkg/labels/registry"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("labelsprovider", New)
	plugin.RegisterNamespace("grpc.services.labelsprovider.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver  string                    `mapstructure:"driver"`
	Drivers map[string]map[string]any `mapstructure:"drivers"`
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "memory"
	}
}

type service struct {
	conf *config
	mgr  labels.Manager
}

func getManager(c *config) (labels.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

// New returns a new LabelsProviderService.
func New(ctx context.Context, m map[string]any) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	mgr, err := getManager(&c)
	if err != nil {
		return nil, err
	}

	return &service{
		conf: &c,
		mgr:  mgr,
	}, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	labelsv1beta1.RegisterLabelsAPIServer(ss, s)
}

func (s *service) AddLabel(ctx context.Context, req *labelsv1beta1.AddLabelRequest) (*labelsv1beta1.AddLabelResponse, error) {
	ref := req.GetRef()
	if ref == nil || ref.GetResourceId() == nil {
		return &labelsv1beta1.AddLabelResponse{
			Status: status.NewInvalidArg(ctx, "ref with resource_id is required"),
		}, nil
	}

	err := s.mgr.SetLabel(ctx, req.GetLabel(), ref.GetResourceId())
	if err != nil {
		return &labelsv1beta1.AddLabelResponse{
			Status: status.NewInternal(ctx, err, "error setting label"),
		}, nil
	}

	return &labelsv1beta1.AddLabelResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) RemoveLabel(ctx context.Context, req *labelsv1beta1.RemoveLabelRequest) (*labelsv1beta1.RemoveLabelResponse, error) {
	ref := req.GetRef()
	if ref == nil || ref.GetResourceId() == nil {
		return &labelsv1beta1.RemoveLabelResponse{
			Status: status.NewInvalidArg(ctx, "ref with resource_id is required"),
		}, nil
	}

	err := s.mgr.UnsetLabel(ctx, req.GetLabel(), ref.GetResourceId())
	if err != nil {
		return &labelsv1beta1.RemoveLabelResponse{
			Status: status.NewInternal(ctx, err, "error removing label"),
		}, nil
	}

	return &labelsv1beta1.RemoveLabelResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) ListLabels(ctx context.Context, req *labelsv1beta1.ListLabelsRequest) (*labelsv1beta1.ListLabelsResponse, error) {
	lbls, err := s.mgr.ListLabels(ctx)
	if err != nil {
		return &labelsv1beta1.ListLabelsResponse{
			Status: status.NewInternal(ctx, err, "error listing labels"),
		}, nil
	}

	return &labelsv1beta1.ListLabelsResponse{
		Status: status.NewOK(ctx),
		Labels: lbls,
	}, nil
}

func (s *service) ListResourcesForLabel(ctx context.Context, req *labelsv1beta1.ListResourcesForLabelRequest) (*labelsv1beta1.ListResourcesForLabelResponse, error) {
	resourceIds, err := s.mgr.ListResourcesForLabel(ctx, req.GetLabels())
	if err != nil {
		return &labelsv1beta1.ListResourcesForLabelResponse{
			Status: status.NewInternal(ctx, err, "error listing resources for label"),
		}, nil
	}

	refs := make([]*provider.Reference, 0, len(resourceIds))
	for _, id := range resourceIds {
		refs = append(refs, &provider.Reference{
			ResourceId: id,
		})
	}

	return &labelsv1beta1.ListResourcesForLabelResponse{
		Status: status.NewOK(ctx),
		Ref:    refs,
	}, nil
}
