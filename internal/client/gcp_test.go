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
	"errors"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-gcp/config"
	"github.com/cloudbase/garm-provider-gcp/internal/spec"
	"github.com/cloudbase/garm-provider-gcp/internal/util"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"
)

func TestCreateInstanceLinux(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpCli := &GcpCli{
		cfg: &config.Config{
			Zone:             "europe-west1-d",
			ProjectId:        "my-project",
			NetworkID:        "my-network",
			SubnetworkID:     "my-subnetwork",
			CredentialsFile:  "path/to/credentials.json",
			ExternalIPAccess: true,
		},
		client: mockClient,
	}

	mockOperation := &compute.Operation{}
	mockClient.On("Insert", mock.Anything, mock.Anything, mock.Anything).Return(mockOperation, nil)
	spec.DefaultCloudConfigFunc = func(bootstrapParams params.BootstrapInstance, tools params.RunnerApplicationDownload, runnerName string) (string, error) {
		return "MockUserData", nil
	}

	spec := &spec.RunnerSpec{
		Zone: "europe-west1-d",
		Tools: params.RunnerApplicationDownload{
			OS:           proto.String("linux"),
			Architecture: proto.String("amd64"),
			DownloadURL:  proto.String("MockURL"),
			Filename:     proto.String("garm-runner"),
		},
		NetworkID:      "my-network",
		SubnetworkID:   "my-subnetwork",
		ControllerID:   "my-controller",
		NicType:        "VIRTIO_NET",
		DiskSize:       50,
		CustomLabels:   map[string]string{"key1": "value1"},
		NetworkTags:    []string{"tag1", "tag2"},
		SourceSnapshot: "projects/garm-testing/global/snapshots/garm-snapshot",
		BootstrapParams: params.BootstrapInstance{
			Name:   "garm-instance",
			Flavor: "n1-standard-1",
			Image:  "projects/garm-testing/global/images/garm-image",
			OSType: params.Linux,
			OSArch: "amd64",
		},
	}

	expectedInstance := &computepb.Instance{
		Name: proto.String("garm-instance"),
		Metadata: &computepb.Metadata{
			Items: []*computepb.Items{
				{
					Key:   proto.String("runner_name"),
					Value: proto.String("garm-instance"),
				},
				{
					Key:   proto.String(linuxUserData),
					Value: proto.String("MockUserData"),
				},
			},
		},
	}
	result, err := gcpCli.CreateInstance(ctx, spec)
	assert.NoError(t, err)
	assert.Equal(t, expectedInstance.Name, result.Name)
	for key, value := range expectedInstance.Metadata.Items {
		assert.Equal(t, *expectedInstance.Metadata.Items[key].Key, *value.Key)
		assert.Equal(t, *expectedInstance.Metadata.Items[key].Value, *value.Value)
	}
	mockClient.AssertExpectations(t)
}

func TestCreateInstanceSpotScheduling(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error { return nil }
	gcpCli := &GcpCli{
		cfg:    &config.Config{Zone: "us-central1-a", ProjectId: "my-project", NetworkID: "my-network", SubnetworkID: "my-subnetwork"},
		client: mockClient,
	}
	mockClient.On("Insert", mock.Anything, mock.MatchedBy(func(req *computepb.InsertInstanceRequest) bool {
		scheduling := req.InstanceResource.GetScheduling()
		return scheduling.GetProvisioningModel() == "SPOT" && scheduling.GetInstanceTerminationAction() == "DELETE" &&
			!scheduling.GetAutomaticRestart() && scheduling.GetOnHostMaintenance() == "TERMINATE"
	}), mock.Anything).Return(&compute.Operation{}, nil).Once()
	spec.DefaultCloudConfigFunc = func(params.BootstrapInstance, params.RunnerApplicationDownload, string) (string, error) {
		return "MockUserData", nil
	}
	_, err := gcpCli.CreateInstance(ctx, minimalRunnerSpec("SPOT", false))
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestCreateInstanceFallsBackToStandardOnlyForCapacity(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error { return nil }
	gcpCli := &GcpCli{
		cfg:    &config.Config{Zone: "us-central1-a", ProjectId: "my-project", NetworkID: "my-network", SubnetworkID: "my-subnetwork"},
		client: mockClient,
	}
	mockClient.On("Insert", mock.Anything, mock.MatchedBy(func(req *computepb.InsertInstanceRequest) bool {
		return req.InstanceResource.GetScheduling().GetProvisioningModel() == "SPOT"
	}), mock.Anything).Return((*compute.Operation)(nil), errors.New("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
	mockClient.On("Insert", mock.Anything, mock.MatchedBy(func(req *computepb.InsertInstanceRequest) bool {
		return req.InstanceResource.GetScheduling() == nil
	}), mock.Anything).Return(&compute.Operation{}, nil).Once()
	spec.DefaultCloudConfigFunc = func(params.BootstrapInstance, params.RunnerApplicationDownload, string) (string, error) {
		return "MockUserData", nil
	}
	_, err := gcpCli.CreateInstance(ctx, minimalRunnerSpec("SPOT", true))
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestCreateInstanceDoesNotFallbackForNonCapacityErrors(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	gcpCli := &GcpCli{
		cfg:    &config.Config{Zone: "us-central1-a", ProjectId: "my-project", NetworkID: "my-network", SubnetworkID: "my-subnetwork"},
		client: mockClient,
	}
	mockClient.On("Insert", mock.Anything, mock.Anything, mock.Anything).Return((*compute.Operation)(nil), errors.New("QUOTA_EXCEEDED")).Once()
	spec.DefaultCloudConfigFunc = func(params.BootstrapInstance, params.RunnerApplicationDownload, string) (string, error) {
		return "MockUserData", nil
	}
	_, err := gcpCli.CreateInstance(ctx, minimalRunnerSpec("SPOT", true))
	assert.ErrorContains(t, err, "QUOTA_EXCEEDED")
	mockClient.AssertNumberOfCalls(t, "Insert", 1)
}

func minimalRunnerSpec(provisioningModel string, fallback bool) *spec.RunnerSpec {
	return &spec.RunnerSpec{
		ProvisioningModel: provisioningModel, FallbackToStandard: fallback,
		NetworkID: "my-network", SubnetworkID: "my-subnetwork", NicType: "GVNIC", DiskSize: 100,
		CustomLabels: map[string]string{"purpose": "ci-runner"},
		BootstrapParams: params.BootstrapInstance{
			Name: "garm-instance", Flavor: "t2a-standard-1",
			Image:  "projects/my-project/global/images/family/ci-runner-2404-arm64",
			OSType: params.Linux, OSArch: params.Arm64,
		},
	}
}

func TestIsCapacityError(t *testing.T) {
	for _, test := range []struct {
		message string
		want    bool
	}{
		{"ZONE_RESOURCE_POOL_EXHAUSTED", true}, {"resourcePoolExhausted", true},
		{"resourceNotReady", true}, {"QUOTA_EXCEEDED", false}, {"PERMISSION_DENIED", false},
	} {
		assert.Equal(t, test.want, isCapacityError(errors.New(test.message)), test.message)
	}
}

func TestCreateInstanceWindows(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpCli := &GcpCli{
		cfg: &config.Config{
			Zone:             "europe-west1-d",
			ProjectId:        "my-project",
			NetworkID:        "my-network",
			SubnetworkID:     "my-subnetwork",
			CredentialsFile:  "path/to/credentials.json",
			ExternalIPAccess: true,
		},
		client: mockClient,
	}

	mockOperation := &compute.Operation{}
	mockClient.On("Insert", mock.Anything, mock.Anything, mock.Anything).Return(mockOperation, nil)
	spec.DefaultRunnerInstallScriptFunc = func(bootstrapParams params.BootstrapInstance, tools params.RunnerApplicationDownload, runnerName string) ([]byte, error) {
		return []byte("MockUserData"), nil
	}

	spec := &spec.RunnerSpec{
		Zone: "europe-west1-d",
		Tools: params.RunnerApplicationDownload{
			OS:           proto.String("windows"),
			Architecture: proto.String("amd64"),
			DownloadURL:  proto.String("MockURL"),
			Filename:     proto.String("garm-runner"),
		},
		NetworkID:      "my-network",
		SubnetworkID:   "my-subnetwork",
		ControllerID:   "my-controller",
		NicType:        "VIRTIO_NET",
		DiskSize:       50,
		CustomLabels:   map[string]string{"key1": "value1"},
		NetworkTags:    []string{"tag1", "tag2"},
		SourceSnapshot: "projects/garm-testing/global/snapshots/garm-snapshot",
		SSHKeys:        "MockSSHKey",
		BootstrapParams: params.BootstrapInstance{
			Name:   "garm-instance",
			Flavor: "n1-standard-1",
			Image:  "projects/garm-testing/global/images/garm-image",
			OSType: params.Windows,
			OSArch: "amd64",
		},
	}

	expectedInstance := &computepb.Instance{
		Name: proto.String("garm-instance"),
		Metadata: &computepb.Metadata{
			Items: []*computepb.Items{
				{
					Key:   proto.String("runner_name"),
					Value: proto.String("garm-instance"),
				},
				{
					Key:   proto.String(windowsStartupScript),
					Value: proto.String("MockUserData"),
				},
				{
					Key:   proto.String("ssh-keys"),
					Value: proto.String("MockSSHKey"),
				},
				{
					Key:   proto.String("enable-windows-ssh"),
					Value: proto.String("TRUE"),
				},
			},
		},
	}
	result, err := gcpCli.CreateInstance(ctx, spec)
	assert.NoError(t, err)
	assert.Equal(t, expectedInstance.Name, result.Name)
	for key, value := range expectedInstance.Metadata.Items {
		assert.Equal(t, *expectedInstance.Metadata.Items[key].Key, *value.Key)
		assert.Equal(t, *expectedInstance.Metadata.Items[key].Value, *value.Value)
	}
	mockClient.AssertExpectations(t)
}

func TestGetInstance(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	gcpCli := &GcpCli{
		cfg: &config.Config{
			Zone:             "europe-west1-d",
			ProjectId:        "my-project",
			NetworkID:        "my-network",
			SubnetworkID:     "my-subnetwork",
			CredentialsFile:  "path/to/credentials.json",
			ExternalIPAccess: true,
		},
		client: mockClient,
	}

	instanceName := "garm-instance"
	expectedInstance := &computepb.Instance{
		Name:   proto.String(instanceName),
		Status: proto.String("RUNNING"),
	}

	mockClient.On("Get", ctx, &computepb.GetInstanceRequest{
		Project:  gcpCli.cfg.ProjectId,
		Zone:     gcpCli.cfg.Zone,
		Instance: util.GetInstanceName(instanceName),
	}, mock.Anything).Return(expectedInstance, nil)

	resultInstance, err := gcpCli.GetInstance(ctx, instanceName)
	assert.NoError(t, err)
	assert.Equal(t, expectedInstance, resultInstance)

	mockClient.AssertExpectations(t)
}

func TestListDescribedInstances(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	gcpCli := &GcpCli{
		cfg: &config.Config{
			Zone:             "europe-west1-d",
			ProjectId:        "my-project",
			NetworkID:        "my-network",
			SubnetworkID:     "my-subnetwork",
			CredentialsFile:  "path/to/credentials.json",
			ExternalIPAccess: true,
		},
		client: mockClient,
	}
	poolID := "garm-pool"
	expectedInstances := []*computepb.Instance{
		{
			Name:   proto.String("garm-instance-1"),
			Status: proto.String("RUNNING"),
			Labels: map[string]string{
				"garmpoolid": poolID,
			},
		},
		{
			Name:   proto.String("garm-instance-2"),
			Status: proto.String("RUNNING"),
			Labels: map[string]string{
				"garmpoolid": poolID,
			},
		},
		{
			Name:   proto.String("garm-instance-3"),
			Status: proto.String("TERMINATED"),
			Labels: map[string]string{
				"garmpoolid": poolID,
			},
		},
		{
			Name:   proto.String("garm-instance-4"),
			Status: proto.String("RUNNING"),
		},
	}
	it := 0
	NextIt = func(*compute.InstanceIterator) (*computepb.Instance, error) {
		if it < len(expectedInstances) {
			it++
			return expectedInstances[it-1], nil
		}
		return nil, nil
	}

	mockClient.On("List", ctx, &computepb.ListInstancesRequest{
		Project: gcpCli.cfg.ProjectId,
		Zone:    gcpCli.cfg.Zone,
		Filter:  proto.String("labels.garmpoolid=garm-pool"),
	}, mock.Anything).Return(&compute.InstanceIterator{}, nil)

	resultInstances, err := gcpCli.ListDescribedInstances(ctx, poolID)
	assert.NoError(t, err)
	assert.Equal(t, expectedInstances, resultInstances)

}

func TestDeleteInstance(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpCli := &GcpCli{
		cfg: &config.Config{
			Zone:             "europe-west1-d",
			ProjectId:        "my-project",
			NetworkID:        "my-network",
			SubnetworkID:     "my-subnetwork",
			CredentialsFile:  "path/to/credentials.json",
			ExternalIPAccess: true,
		},
		client: mockClient,
	}

	instanceName := "garm-instance"
	mockOperation := &compute.Operation{}
	mockClient.On("Delete", ctx, &computepb.DeleteInstanceRequest{
		Project:  gcpCli.cfg.ProjectId,
		Zone:     gcpCli.cfg.Zone,
		Instance: util.GetInstanceName(instanceName),
	}, mock.Anything).Return(mockOperation, nil)

	err := gcpCli.DeleteInstance(ctx, instanceName)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestDeleteInstanceNotFound(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpCli := &GcpCli{
		cfg: &config.Config{
			Zone:             "europe-west1-d",
			ProjectId:        "my-project",
			NetworkID:        "my-network",
			SubnetworkID:     "my-subnetwork",
			CredentialsFile:  "path/to/credentials.json",
			ExternalIPAccess: true,
		},
		client: mockClient,
	}

	instanceName := "garm-instance"
	mockOperation := &compute.Operation{}
	mockErr, _ := apierror.FromError(&googleapi.Error{
		Code: 404,
	})
	mockClient.On("Delete", ctx, &computepb.DeleteInstanceRequest{
		Project:  gcpCli.cfg.ProjectId,
		Zone:     gcpCli.cfg.Zone,
		Instance: util.GetInstanceName(instanceName),
	}, mock.Anything).Return(mockOperation, mockErr)

	err := gcpCli.DeleteInstance(ctx, instanceName)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestDeleteInstanceError(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpCli := &GcpCli{
		cfg: &config.Config{
			Zone:             "europe-west1-d",
			ProjectId:        "my-project",
			NetworkID:        "my-network",
			SubnetworkID:     "my-subnetwork",
			CredentialsFile:  "path/to/credentials.json",
			ExternalIPAccess: true,
		},
		client: mockClient,
	}

	instanceName := "garm-instance"
	mockOperation := &compute.Operation{}
	mockErr, _ := apierror.FromError(&googleapi.Error{
		Code: 403,
	})
	mockClient.On("Delete", ctx, &computepb.DeleteInstanceRequest{
		Project:  gcpCli.cfg.ProjectId,
		Zone:     gcpCli.cfg.Zone,
		Instance: util.GetInstanceName(instanceName),
	}, mock.Anything).Return(mockOperation, mockErr)

	err := gcpCli.DeleteInstance(ctx, instanceName)
	assert.Error(t, err)

	mockClient.AssertExpectations(t)
}

func TestStopInstance(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpCli := &GcpCli{
		cfg: &config.Config{
			Zone:             "europe-west1-d",
			ProjectId:        "my-project",
			NetworkID:        "my-network",
			SubnetworkID:     "my-subnetwork",
			CredentialsFile:  "path/to/credentials.json",
			ExternalIPAccess: true,
		},
		client: mockClient,
	}

	instanceName := "garm-instance"
	mockOperation := &compute.Operation{}
	mockClient.On("Stop", ctx, &computepb.StopInstanceRequest{
		Project:  gcpCli.cfg.ProjectId,
		Zone:     gcpCli.cfg.Zone,
		Instance: util.GetInstanceName(instanceName),
	}, mock.Anything).Return(mockOperation, nil)

	err := gcpCli.StopInstance(ctx, instanceName)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestStartInstance(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockGcpClient)
	WaitOp = func(op *compute.Operation, ctx context.Context, opts ...gax.CallOption) error {
		return nil
	}
	gcpCli := &GcpCli{
		cfg: &config.Config{
			Zone:             "europe-west1-d",
			ProjectId:        "my-project",
			NetworkID:        "my-network",
			SubnetworkID:     "my-subnetwork",
			CredentialsFile:  "path/to/credentials.json",
			ExternalIPAccess: true,
		},
		client: mockClient,
	}

	instanceName := "garm-instance"
	mockOperation := &compute.Operation{}
	mockClient.On("Start", ctx, &computepb.StartInstanceRequest{
		Project:  gcpCli.cfg.ProjectId,
		Zone:     gcpCli.cfg.Zone,
		Instance: util.GetInstanceName(instanceName),
	}, mock.Anything).Return(mockOperation, nil)

	err := gcpCli.StartInstance(ctx, instanceName)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}
