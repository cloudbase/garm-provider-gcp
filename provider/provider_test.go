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

package provider

import (
	"context"
	"encoding/json"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-gcp/config"
	"github.com/cloudbase/garm-provider-gcp/internal/client"
	"github.com/cloudbase/garm-provider-gcp/internal/spec"
	"github.com/cloudbase/garm-provider-gcp/internal/util"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/proto"
)

func TestCreateInstance(t *testing.T) {
	ctx := context.Background()
	mockClient := new(client.MockGcpClient)
	spec.DefaultToolFetch = func(osType params.OSType, osArch params.OSArch, tools []params.RunnerApplicationDownload) (params.RunnerApplicationDownload, error) {
		return params.RunnerApplicationDownload{
			OS:           proto.String("linux"),
			Architecture: proto.String("amd64"),
			DownloadURL:  proto.String("MockURL"),
			Filename:     proto.String("garm-runner"),
		}, nil
	}
	client.WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpProvider := &GcpProvider{
		gcpCli:       &client.GcpCli{},
		controllerID: "my-controller",
	}
	config := config.Config{
		Zone:             "europe-west1-d",
		ProjectId:        "my-project",
		NetworkID:        "my-network",
		SubnetworkID:     "my-subnetwork",
		CredentialsFile:  "path/to/credentials.json",
		ExternalIPAccess: true,
	}
	gcpProvider.gcpCli.SetClient(mockClient)
	gcpProvider.gcpCli.SetConfig(&config)

	mockOperation := &compute.Operation{}
	mockClient.On("Insert", mock.Anything, mock.Anything, mock.Anything).Return(mockOperation, nil)
	bootstrapParams := params.BootstrapInstance{
		Name:   "garm-instance",
		Flavor: "n1-standard-1",
		Image:  "projects/garm-testing/global/images/garm-image",
		Tools: []params.RunnerApplicationDownload{
			{
				OS:           proto.String("linux"),
				Architecture: proto.String("amd64"),
				DownloadURL:  proto.String("MockURL"),
				Filename:     proto.String("garm-runner"),
			},
		},
		OSType:     params.Linux,
		OSArch:     params.Amd64,
		PoolID:     "my-pool",
		ExtraSpecs: json.RawMessage(`{}`),
	}
	expectedInstance := params.ProviderInstance{
		ProviderID: "garm-instance",
		Name:       "garm-instance",
		OSType:     "linux",
		OSArch:     "amd64",
		Status:     "running",
	}

	result, err := gcpProvider.CreateInstance(ctx, bootstrapParams)
	assert.NoError(t, err)
	assert.Equal(t, expectedInstance, result)

}

func TestGetInstance(t *testing.T) {
	ctx := context.Background()
	mockClient := new(client.MockGcpClient)
	gcpProvider := &GcpProvider{
		gcpCli:       &client.GcpCli{},
		controllerID: "my-controller",
	}
	config := config.Config{
		Zone:             "europe-west1-d",
		ProjectId:        "my-project",
		NetworkID:        "my-network",
		SubnetworkID:     "my-subnetwork",
		CredentialsFile:  "path/to/credentials.json",
		ExternalIPAccess: true,
	}
	gcpProvider.gcpCli.SetClient(mockClient)
	gcpProvider.gcpCli.SetConfig(&config)

	instanceName := "my-instance"
	expectedInstance := &computepb.Instance{
		Name:   proto.String(instanceName),
		Labels: map[string]string{"ostype": "linux"},
		Disks:  []*computepb.AttachedDisk{{Architecture: proto.String("amd64")}},
		Status: proto.String("RUNNING"),
	}
	expectedInstanceParams := params.ProviderInstance{
		ProviderID: "my-instance",
		Name:       "my-instance",
		OSType:     "linux",
		OSArch:     "amd64",
		Status:     "running",
	}

	mockClient.On("Get", ctx, &computepb.GetInstanceRequest{
		Project:  gcpProvider.gcpCli.Config().ProjectId,
		Zone:     gcpProvider.gcpCli.Config().Zone,
		Instance: util.GetInstanceName(instanceName),
	}, mock.Anything).Return(expectedInstance, nil)

	result, err := gcpProvider.GetInstance(ctx, instanceName)
	assert.NoError(t, err)
	assert.Equal(t, expectedInstanceParams, result)
	mockClient.AssertExpectations(t)

}

func TestDeleteInstance(t *testing.T) {
	ctx := context.Background()
	mockClient := new(client.MockGcpClient)
	mockOperation := &compute.Operation{}
	client.WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpProvider := &GcpProvider{
		gcpCli:       &client.GcpCli{},
		controllerID: "my-controller",
	}
	config := config.Config{
		Zone:             "europe-west1-d",
		ProjectId:        "my-project",
		NetworkID:        "my-network",
		SubnetworkID:     "my-subnetwork",
		CredentialsFile:  "path/to/credentials.json",
		ExternalIPAccess: true,
	}
	gcpProvider.gcpCli.SetClient(mockClient)
	gcpProvider.gcpCli.SetConfig(&config)

	instanceName := "my-instance"
	mockClient.On("Delete", ctx, mock.AnythingOfType("*computepb.DeleteInstanceRequest"), []gax.CallOption(nil)).Return(mockOperation, nil)

	err := gcpProvider.DeleteInstance(ctx, instanceName)

	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestListInstances(t *testing.T) {
	ctx := context.Background()
	mockClient := new(client.MockGcpClient)
	poolID := "garm-pool"
	gcpProvider := &GcpProvider{
		gcpCli:       &client.GcpCli{},
		controllerID: "my-controller",
	}
	config := config.Config{
		Zone:             "europe-west1-d",
		ProjectId:        "my-project",
		NetworkID:        "my-network",
		SubnetworkID:     "my-subnetwork",
		CredentialsFile:  "path/to/credentials.json",
		ExternalIPAccess: true,
	}
	gcpProvider.gcpCli.SetClient(mockClient)
	gcpProvider.gcpCli.SetConfig(&config)
	toBeIteratedInstances := []*computepb.Instance{
		{
			Name: proto.String("garm-instance-1"),

			Status: proto.String("RUNNING"),
			Labels: map[string]string{
				"garmpoolid": poolID,
				"ostype":     "linux",
			},
			Disks: []*computepb.AttachedDisk{{Architecture: proto.String("amd64")}},
		},
		{
			Name:   proto.String("garm-instance-2"),
			Status: proto.String("RUNNING"),
			Labels: map[string]string{
				"garmpoolid": poolID,
				"ostype":     "linux",
			},
			Disks: []*computepb.AttachedDisk{{Architecture: proto.String("amd64")}},
		},
		{
			Name:   proto.String("garm-instance-3"),
			Status: proto.String("TERMINATED"),
			Labels: map[string]string{
				"garmpoolid": poolID,
				"ostype":     "linux",
			},
			Disks: []*computepb.AttachedDisk{{Architecture: proto.String("amd64")}},
		},
		{
			Name:   proto.String("garm-instance-4"),
			Status: proto.String("RUNNING"),
			Labels: map[string]string{
				"garmpoolid": poolID,
				"ostype":     "linux",
			},
			Disks: []*computepb.AttachedDisk{{Architecture: proto.String("amd64")}},
		},
	}
	expectedInstances := []params.ProviderInstance{
		{
			ProviderID: "garm-instance-1",
			Name:       "garm-instance-1",
			OSType:     "linux",
			OSArch:     "amd64",
			Status:     "running",
		},
		{
			ProviderID: "garm-instance-2",
			Name:       "garm-instance-2",
			OSType:     "linux",
			OSArch:     "amd64",
			Status:     "running",
		},
		{
			ProviderID: "garm-instance-3",
			Name:       "garm-instance-3",
			OSType:     "linux",
			OSArch:     "amd64",
			Status:     "stopped",
		},
		{
			ProviderID: "garm-instance-4",
			Name:       "garm-instance-4",
			OSType:     "linux",
			OSArch:     "amd64",
			Status:     "running",
		},
	}

	it := 0
	client.NextIt = func(*compute.InstanceIterator) (*computepb.Instance, error) {
		if it < len(toBeIteratedInstances) {
			it++
			return toBeIteratedInstances[it-1], nil
		}
		return nil, nil
	}

	mockClient.On("List", ctx, &computepb.ListInstancesRequest{
		Project: gcpProvider.gcpCli.Config().ProjectId,
		Zone:    gcpProvider.gcpCli.Config().Zone,
		Filter:  proto.String("labels.garmpoolid=garm-pool"),
	}, mock.Anything).Return(&compute.InstanceIterator{}, nil)

	resultInstances, err := gcpProvider.ListInstances(ctx, poolID)
	assert.NoError(t, err)
	assert.Equal(t, expectedInstances, resultInstances)

}

func TestStop(t *testing.T) {
	ctx := context.Background()
	mockClient := new(client.MockGcpClient)
	mockOperation := &compute.Operation{}
	client.WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpProvider := &GcpProvider{
		gcpCli:       &client.GcpCli{},
		controllerID: "my-controller",
	}
	config := config.Config{
		Zone:             "europe-west1-d",
		ProjectId:        "my-project",
		NetworkID:        "my-network",
		SubnetworkID:     "my-subnetwork",
		CredentialsFile:  "path/to/credentials.json",
		ExternalIPAccess: true,
	}
	gcpProvider.gcpCli.SetClient(mockClient)
	gcpProvider.gcpCli.SetConfig(&config)

	instanceName := "my-instance"
	mockClient.On("Stop", ctx, mock.AnythingOfType("*computepb.StopInstanceRequest"), []gax.CallOption(nil)).Return(mockOperation, nil)

	err := gcpProvider.Stop(ctx, instanceName, false)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestStart(t *testing.T) {
	ctx := context.Background()
	mockClient := new(client.MockGcpClient)
	mockOperation := &compute.Operation{}
	client.WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpProvider := &GcpProvider{
		gcpCli:       &client.GcpCli{},
		controllerID: "my-controller",
	}
	config := config.Config{
		Zone:             "europe-west1-d",
		ProjectId:        "my-project",
		NetworkID:        "my-network",
		SubnetworkID:     "my-subnetwork",
		CredentialsFile:  "path/to/credentials.json",
		ExternalIPAccess: true,
	}
	gcpProvider.gcpCli.SetClient(mockClient)
	gcpProvider.gcpCli.SetConfig(&config)

	instanceName := "my-instance"
	mockClient.On("Start", ctx, mock.AnythingOfType("*computepb.StartInstanceRequest"), []gax.CallOption(nil)).Return(mockOperation, nil)

	err := gcpProvider.Start(ctx, instanceName)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}
