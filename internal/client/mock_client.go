// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Cloudbase Solutions SRL
//
//	Licensed under the Apache License, Version 2.0 (the "License"); you may
//	not use this file except in compliance with the License. You may obtain
//	a copy of the License at
//
//	     http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//	WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//	License for the specific language governing permissions and limitations
//	under the License.

package client

import (
	"context"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/mock"
)

// MockGcpClient is a mock of the ClientInterface
type MockGcpClient struct {
	mock.Mock
}

func (m *MockGcpClient) Insert(ctx context.Context, req *computepb.InsertInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error) {
	args := m.Called(ctx, req, opts)
	return args.Get(0).(*compute.Operation), args.Error(1)
}

func (m *MockGcpClient) Start(ctx context.Context, req *computepb.StartInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error) {
	args := m.Called(ctx, req, opts)
	return args.Get(0).(*compute.Operation), args.Error(1)
}

func (m *MockGcpClient) Stop(ctx context.Context, req *computepb.StopInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error) {
	args := m.Called(ctx, req, opts)
	return args.Get(0).(*compute.Operation), args.Error(1)
}

func (m *MockGcpClient) Delete(ctx context.Context, req *computepb.DeleteInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error) {
	args := m.Called(ctx, req, opts)
	return args.Get(0).(*compute.Operation), args.Error(1)
}

func (m *MockGcpClient) List(ctx context.Context, req *computepb.ListInstancesRequest, opts ...gax.CallOption) *compute.InstanceIterator {
	args := m.Called(ctx, req, opts)
	return args.Get(0).(*compute.InstanceIterator)
}

func (m *MockGcpClient) Get(ctx context.Context, req *computepb.GetInstanceRequest, opts ...gax.CallOption) (*computepb.Instance, error) {
	args := m.Called(ctx, req, opts)
	return args.Get(0).(*computepb.Instance), args.Error(1)
}
