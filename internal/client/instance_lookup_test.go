// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Cloudbase Solutions SRL
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package client

import (
	"context"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-gcp/config"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

func TestGetInstanceUsesZonePrefixedProviderID(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient := lookupTestClient()
	expected := createdPolicyInstance("us-central1-b")
	mockClient.On("Get", ctx, &computepb.GetInstanceRequest{
		Project: "example-project", Zone: "us-central1-b", Instance: "garm-instance",
	}, mock.Anything).Return(expected, nil).Once()

	instance, err := gcpCli.GetInstance(ctx, "us-central1-b/garm-instance")
	require.NoError(t, err)
	assert.Equal(t, expected, instance)
	mockClient.AssertExpectations(t)
}

func TestGetInstanceFallsBackForLegacyBareProviderID(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient := lookupTestClient()
	mockClient.On("Get", ctx, &computepb.GetInstanceRequest{
		Project: "example-project", Zone: "us-central1-a", Instance: "garm-instance",
	}, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()
	mockClient.On("AggregatedList", ctx, mock.MatchedBy(func(req *computepb.AggregatedListInstancesRequest) bool {
		return req.Project == "example-project" && req.GetFilter() == `name = "garm-instance"`
	}), mock.Anything).Return(&compute.InstancesScopedListPairIterator{}).Once()
	instance := createdPolicyInstance("us-central1-b")
	setAggregatedResults(t, compute.InstancesScopedListPair{
		Key: "zones/us-central1-b", Value: &computepb.InstancesScopedList{Instances: []*computepb.Instance{instance}},
	})

	result, err := gcpCli.GetInstance(ctx, "garm-instance")
	require.NoError(t, err)
	assert.Equal(t, "zones/us-central1-b", result.GetZone())
	mockClient.AssertExpectations(t)
}

func TestDeleteInstanceFallsBackForLegacyBareProviderID(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient := lookupTestClient()
	previousWaitOp := WaitOp
	WaitOp = func(*compute.Operation, context.Context, ...gax.CallOption) error { return nil }
	t.Cleanup(func() { WaitOp = previousWaitOp })
	mockClient.On("Delete", ctx, &computepb.DeleteInstanceRequest{
		Project: "example-project", Zone: "us-central1-a", Instance: "garm-instance",
	}, mock.Anything).Return((*compute.Operation)(nil), notFoundError()).Once()
	mockClient.On("AggregatedList", ctx, mock.Anything, mock.Anything).Return(&compute.InstancesScopedListPairIterator{}).Once()
	setAggregatedResults(t, compute.InstancesScopedListPair{
		Key:   "zones/us-central1-b",
		Value: &computepb.InstancesScopedList{Instances: []*computepb.Instance{{Name: proto.String("garm-instance")}}},
	})
	mockClient.On("Delete", ctx, &computepb.DeleteInstanceRequest{
		Project: "example-project", Zone: "us-central1-b", Instance: "garm-instance",
	}, mock.Anything).Return(&compute.Operation{}, nil).Once()

	require.NoError(t, gcpCli.DeleteInstance(ctx, "garm-instance"))
	mockClient.AssertExpectations(t)
}

func TestDeleteInstanceUsesZonePrefixedProviderID(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient := lookupTestClient()
	previousWaitOp := WaitOp
	WaitOp = func(*compute.Operation, context.Context, ...gax.CallOption) error { return nil }
	t.Cleanup(func() { WaitOp = previousWaitOp })
	mockClient.On("Delete", ctx, &computepb.DeleteInstanceRequest{
		Project: "example-project", Zone: "us-central1-f", Instance: "garm-instance",
	}, mock.Anything).Return(&compute.Operation{}, nil).Once()

	require.NoError(t, gcpCli.DeleteInstance(ctx, "us-central1-f/garm-instance"))
	mockClient.AssertNotCalled(t, "AggregatedList", mock.Anything, mock.Anything, mock.Anything)
}

func lookupTestClient() (*GcpCli, *MockGcpClient) {
	mockClient := new(MockGcpClient)
	return &GcpCli{
		cfg:    &config.Config{ProjectId: "example-project", Zone: "us-central1-a"},
		client: mockClient,
	}, mockClient
}

func setAggregatedResults(t *testing.T, results ...compute.InstancesScopedListPair) {
	t.Helper()
	index := 0
	previous := NextAggregatedIt
	NextAggregatedIt = func(*compute.InstancesScopedListPairIterator) (compute.InstancesScopedListPair, error) {
		if index < len(results) {
			result := results[index]
			index++
			return result, nil
		}
		return compute.InstancesScopedListPair{}, iterator.Done
	}
	t.Cleanup(func() { NextAggregatedIt = previous })
}
