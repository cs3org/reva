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

package gateway

import (
	"context"
	"fmt"

	labels "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/errors"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/service"
)

// resolveRef ensures the reference has a ResourceId by statting it if necessary.
func (s *svc) resolveRef(ctx context.Context, ref *provider.Reference) (*provider.Reference, *rpc.Status, error) {
	if ref == nil {
		return nil, status.NewInvalidArg(ctx, "ref is required"), nil
	}
	if ref.GetResourceId() != nil {
		return ref, nil, nil
	}

	statRes, err := s.Stat(ctx, &provider.StatRequest{Ref: ref})
	if err != nil {
		return nil, nil, errors.Wrap(err, "gateway: error statting ref")
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		return nil, statRes.Status, nil
	}

	return &provider.Reference{ResourceId: statRes.Info.Id}, nil, nil
}

func labelMetadataKey(uid, label string) string {
	return fmt.Sprintf("reva.labels.%s.%s", uid, label)
}

func (s *svc) AddLabel(ctx context.Context, req *labels.AddLabelRequest) (*labels.AddLabelResponse, error) {
	ref, st, err := s.resolveRef(ctx, req.GetRef())
	if err != nil {
		return &labels.AddLabelResponse{
			Status: status.NewInternal(ctx, err, "error resolving ref"),
		}, nil
	}
	if st != nil {
		return &labels.AddLabelResponse{Status: st}, nil
	}
	req.Ref = ref

	u := appctx.ContextMustGetUser(ctx)

	// Set the label as metadata on the storage provider first.
	metaRes, err := s.SetArbitraryMetadata(ctx, &provider.SetArbitraryMetadataRequest{
		Ref: ref,
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata: map[string]string{
				labelMetadataKey(u.Id.OpaqueId, req.GetLabel()): "1",
			},
		},
	})
	if err != nil {
		return &labels.AddLabelResponse{
			Status: status.NewInternal(ctx, err, "error setting label metadata on storage provider"),
		}, nil
	}
	if metaRes.Status.Code != rpc.Code_CODE_OK {
		return &labels.AddLabelResponse{Status: metaRes.Status}, nil
	}

	// Store the label in the labels database.
	c, err := service.Labels(ctx)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetLabelsClient")
		return &labels.AddLabelResponse{
			Status: status.NewInternal(ctx, err, "error getting labels client"),
		}, nil
	}

	res, err := c.AddLabel(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling AddLabel")
	}

	return res, nil
}

func (s *svc) RemoveLabel(ctx context.Context, req *labels.RemoveLabelRequest) (*labels.RemoveLabelResponse, error) {
	ref, st, err := s.resolveRef(ctx, req.GetRef())
	if err != nil {
		return &labels.RemoveLabelResponse{
			Status: status.NewInternal(ctx, err, "error resolving ref"),
		}, nil
	}
	if st != nil {
		return &labels.RemoveLabelResponse{Status: st}, nil
	}
	req.Ref = ref

	u := appctx.ContextMustGetUser(ctx)

	// Remove the label metadata from the storage provider first.
	metaRes, err := s.UnsetArbitraryMetadata(ctx, &provider.UnsetArbitraryMetadataRequest{
		Ref:                   ref,
		ArbitraryMetadataKeys: []string{labelMetadataKey(u.Id.OpaqueId, req.GetLabel())},
	})
	if err != nil {
		return &labels.RemoveLabelResponse{
			Status: status.NewInternal(ctx, err, "error unsetting label metadata on storage provider"),
		}, nil
	}
	if metaRes.Status.Code != rpc.Code_CODE_OK {
		return &labels.RemoveLabelResponse{Status: metaRes.Status}, nil
	}

	// Remove the label from the labels database.
	c, err := service.Labels(ctx)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetLabelsClient")
		return &labels.RemoveLabelResponse{
			Status: status.NewInternal(ctx, err, "error getting labels client"),
		}, nil
	}

	res, err := c.RemoveLabel(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RemoveLabel")
	}

	return res, nil
}

func (s *svc) ListLabels(ctx context.Context, req *labels.ListLabelsRequest) (*labels.ListLabelsResponse, error) {
	c, err := service.Labels(ctx)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetLabelsClient")
		return &labels.ListLabelsResponse{
			Status: status.NewInternal(ctx, err, "error getting labels client"),
		}, nil
	}

	res, err := c.ListLabels(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListLabels")
	}

	return res, nil
}

func (s *svc) ListResourcesForLabel(ctx context.Context, req *labels.ListResourcesForLabelRequest) (*labels.ListResourcesForLabelResponse, error) {
	c, err := service.Labels(ctx)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetLabelsClient")
		return &labels.ListResourcesForLabelResponse{
			Status: status.NewInternal(ctx, err, "error getting labels client"),
		}, nil
	}

	res, err := c.ListResourcesForLabel(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListResourcesForLabel")
	}

	return res, nil
}
