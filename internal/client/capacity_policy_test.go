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
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
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
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"
)

func TestBuildPlacementAttemptsOrderingAndZoneCompatibility(t *testing.T) {
	policy := &spec.CapacityPolicy{
		Zones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
		Candidates: []spec.CapacityCandidate{
			{MachineType: "t2a-standard-2", Architecture: params.Arm64, Zones: []string{"us-central1-a", "us-central1-b"}},
			{MachineType: "c4a-standard-2", Architecture: params.Arm64},
			{MachineType: "c4a-highcpu-4", Architecture: params.Arm64},
		},
		ProvisioningModels: []string{"SPOT", "STANDARD"},
	}

	attempts := buildPlacementAttempts(policy)
	require.Len(t, attempts, 4)
	assert.Equal(t, "SPOT", attempts[0].model)
	assert.Equal(t, []string{"us-central1-a", "us-central1-b"}, attempts[0].zones)
	assert.Equal(t, []int{0}, candidateRanks(attempts[0].candidates))
	assert.Equal(t, []string{"us-central1-a", "us-central1-b", "us-central1-c"}, attempts[1].zones)
	assert.Equal(t, []int{1, 2}, candidateRanks(attempts[1].candidates))
	assert.Equal(t, "STANDARD", attempts[2].model)
	assert.Equal(t, []int{0}, candidateRanks(attempts[2].candidates))
	assert.Equal(t, []int{1, 2}, candidateRanks(attempts[3].candidates))
}

func TestBuildPlacementAttemptsTreatsCandidateZonesAsASet(t *testing.T) {
	policy := &spec.CapacityPolicy{
		Zones: []string{"us-central1-a", "us-central1-b", "us-central1-c"},
		Candidates: []spec.CapacityCandidate{
			{MachineType: "n2d-standard-4", Architecture: params.Amd64, Zones: []string{"us-central1-b", "us-central1-a"}},
			{MachineType: "n2-standard-4", Architecture: params.Amd64, Zones: []string{"us-central1-a", "us-central1-b"}},
		},
		ProvisioningModels: []string{"STANDARD"},
	}

	attempts := buildPlacementAttempts(policy)
	require.Len(t, attempts, 1)
	assert.Equal(t, []string{"us-central1-a", "us-central1-b"}, attempts[0].zones)
	assert.Equal(t, []int{0, 1}, candidateRanks(attempts[0].candidates))
}

func TestBuildBulkInsertRequest(t *testing.T) {
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Zones = []string{"us-central1-a", "us-central1-b"}
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "n2d-standard-4", Architecture: params.Amd64, Image: "projects/example/global/images/override", DiskType: "hyperdisk-balanced", DiskSize: 150},
		{MachineType: "n2-standard-4", Architecture: params.Amd64},
	}
	inst := basePolicyInstance()
	candidates := []rankedCandidate{
		{candidate: runnerSpec.CapacityPolicy.Candidates[0], rank: 0},
		{candidate: runnerSpec.CapacityPolicy.Candidates[1], rank: 1},
	}

	req, err := buildBulkInsertRequest("example-project", runnerSpec, inst, "SPOT", runnerSpec.CapacityPolicy.Zones, candidates)
	require.NoError(t, err)
	assert.Equal(t, "example-project", req.Project)
	assert.Equal(t, "us-central1", req.Region)
	assert.NotEmpty(t, req.GetRequestId())
	resource := req.GetBulkInsertInstanceResourceResource()
	assert.EqualValues(t, 1, resource.GetCount())
	assert.EqualValues(t, 1, resource.GetMinCount())
	assert.Equal(t, "ANY_SINGLE_ZONE", resource.GetLocationPolicy().GetTargetShape())
	assert.Equal(t, []string{"zones/us-central1-a", "zones/us-central1-b"}, []string{
		resource.GetLocationPolicy().GetZones()[0].GetZone(),
		resource.GetLocationPolicy().GetZones()[1].GetZone(),
	})
	assert.Contains(t, resource.GetPerInstanceProperties(), "garm-instance")
	assert.Empty(t, resource.GetInstanceProperties().GetDisks(), "candidate disks must live only on flexibility selections")
	assert.Equal(t, "SPOT", resource.GetInstanceProperties().GetScheduling().GetProvisioningModel())
	assert.True(t, resource.GetInstanceProperties().GetScheduling().GetPreemptible())
	assert.Equal(t, "DELETE", resource.GetInstanceProperties().GetScheduling().GetInstanceTerminationAction())
	assert.Equal(t, "GVNIC", resource.GetInstanceProperties().GetNetworkInterfaces()[0].GetNicType())
	assert.Equal(t, "runner", resource.GetInstanceProperties().GetLabels()["purpose"])
	assert.Equal(t, "runner@example.invalid", resource.GetInstanceProperties().GetServiceAccounts()[0].GetEmail())
	assert.Equal(t, "#cloud-config", resource.GetInstanceProperties().GetMetadata().GetItems()[0].GetValue())
	assert.Contains(t, resource.GetInstanceProperties().GetMetadata().String(), util.CapacityPolicyMetadataKey)

	primary := resource.GetInstanceFlexibilityPolicy().GetInstanceSelections()["selection-000"]
	secondary := resource.GetInstanceFlexibilityPolicy().GetInstanceSelections()["selection-001"]
	assert.EqualValues(t, 0, primary.GetRank())
	assert.Equal(t, []string{"n2d-standard-4"}, primary.GetMachineTypes())
	assert.Equal(t, "projects/example/global/images/override", primary.GetDisks()[0].GetInitializeParams().GetSourceImage())
	assert.Equal(t, "hyperdisk-balanced", primary.GetDisks()[0].GetInitializeParams().GetDiskType())
	assert.EqualValues(t, 150, primary.GetDisks()[0].GetInitializeParams().GetDiskSizeGb())
	assert.Equal(t, "X86_64", primary.GetDisks()[0].GetInitializeParams().GetArchitecture())
	assert.EqualValues(t, 1, secondary.GetRank())
	assert.Equal(t, runnerSpec.BootstrapParams.Image, secondary.GetDisks()[0].GetInitializeParams().GetSourceImage())
	assert.Equal(t, runnerSpec.DiskType, secondary.GetDisks()[0].GetInitializeParams().GetDiskType())
}

func TestClassifyPlacementError(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		class placementErrorClass
	}{
		{name: "zonal stockout", err: errors.New("ZONE_RESOURCE_POOL_EXHAUSTED"), class: placementErrorCapacity},
		{name: "resource not ready", err: errors.New("resourceNotReady"), class: placementErrorCapacity},
		{name: "quota", err: errors.New("QUOTA_EXCEEDED"), class: placementErrorQuota},
		{name: "authentication", err: errors.New("UNAUTHENTICATED"), class: placementErrorTerminal},
		{name: "permission", err: errors.New("PERMISSION_DENIED"), class: placementErrorTerminal},
		{name: "invalid machine", err: errors.New("Invalid value for field machineType"), class: placementErrorTerminal},
		{name: "invalid image", err: errors.New("source image was not found"), class: placementErrorTerminal},
		{name: "invalid disk", err: errors.New("invalid disk type"), class: placementErrorTerminal},
		{name: "invalid network", err: errors.New("Invalid value for networkInterfaces"), class: placementErrorTerminal},
		{name: "malformed request", err: errors.New("INVALID_ARGUMENT"), class: placementErrorTerminal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.class, classifyPlacementError(test.err))
		})
	}
}

func TestQuotaAdvancesCandidateWithDistinctLog(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "n2d-standard-4", Architecture: params.Amd64},
		{MachineType: "n2-standard-4", Architecture: params.Amd64},
	}
	quotaErr := errors.New("QUOTA_EXCEEDED: N2D_CPUS")
	regional.On("BulkInsert", ctx, mock.MatchedBy(func(req *computepb.BulkInsertRegionInstanceRequest) bool {
		return len(req.GetBulkInsertInstanceResourceResource().GetInstanceFlexibilityPolicy().GetInstanceSelections()) == 2
	}), mock.Anything).Return((*compute.Operation)(nil), quotaErr).Once()
	regional.On("BulkInsert", ctx, mock.MatchedBy(func(req *computepb.BulkInsertRegionInstanceRequest) bool {
		selections := req.GetBulkInsertInstanceResourceResource().GetInstanceFlexibilityPolicy().GetInstanceSelections()
		_, retained := selections["selection-001"]
		return len(selections) == 1 && retained
	}), mock.Anything).Return(&compute.Operation{}, nil).Once()
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()
	created := createdPolicyInstance("us-central1-a")
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return(created, nil).Once()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	result, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.NoError(t, err)
	assert.Equal(t, created, result)
	assert.Contains(t, logs.String(), quotaAdvanceLogMarker)
	assert.Contains(t, logs.String(), "machine_type=n2d-standard-4")
	regional.AssertExpectations(t)
	regional.AssertNumberOfCalls(t, "BulkInsert", 2)
}

func TestCapacityErrorAdvancesProvisioningModel(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}}
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasProvisioningModel("SPOT")), mock.Anything).Return((*compute.Operation)(nil), errors.New("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasProvisioningModel("STANDARD")), mock.Anything).Return(&compute.Operation{}, nil).Once()
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()
	created := createdPolicyInstance("us-central1-a")
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return(created, nil).Once()

	result, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.NoError(t, err)
	assert.Equal(t, "zones/us-central1-a", result.GetZone())
	regional.AssertNumberOfCalls(t, "BulkInsert", 2)
}

func TestCapacityErrorAdvancesZoneCompatibleCandidate(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.BootstrapParams.OSArch = params.Arm64
	runnerSpec.CapacityPolicy.ProvisioningModels = []string{"STANDARD"}
	runnerSpec.CapacityPolicy.Zones = []string{"us-central1-a", "us-central1-b"}
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "t2a-standard-2", Architecture: params.Arm64, Zones: []string{"us-central1-a"}},
		{MachineType: "c4a-standard-2", Architecture: params.Arm64},
	}
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasFirstZone("zones/us-central1-a")), mock.Anything).Return((*compute.Operation)(nil), errors.New("ZONE_RESOURCE_POOL_EXHAUSTED: t2a stockout")).Once()
	regional.On("BulkInsert", ctx, mock.MatchedBy(func(req *computepb.BulkInsertRegionInstanceRequest) bool {
		zones := req.GetBulkInsertInstanceResourceResource().GetLocationPolicy().GetZones()
		return len(zones) == 2 && zones[1].GetZone() == "zones/us-central1-b"
	}), mock.Anything).Return(&compute.Operation{}, nil).Once()
	mockClient.On("Get", ctx, mock.MatchedBy(func(req *computepb.GetInstanceRequest) bool {
		return req.Zone == "us-central1-a"
	}), mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Twice()
	created := createdPolicyInstance("us-central1-b")
	mockClient.On("Get", ctx, mock.MatchedBy(func(req *computepb.GetInstanceRequest) bool {
		return req.Zone == "us-central1-b"
	}), mock.Anything).Return(created, nil).Once()

	result, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.NoError(t, err)
	assert.Equal(t, "zones/us-central1-b", result.GetZone())
	regional.AssertNumberOfCalls(t, "BulkInsert", 2)
}

func TestQuotaDoesNotAdvanceProvisioningModel(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}}
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasProvisioningModel("SPOT")), mock.Anything).Return((*compute.Operation)(nil), errors.New("QUOTA_EXCEEDED")).Once()
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()

	_, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "QUOTA_EXCEEDED")
	regional.AssertNumberOfCalls(t, "BulkInsert", 1)
}

func TestTerminalErrorAggregatesEveryCandidateReason(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.ProvisioningModels = []string{"SPOT"}
	runnerSpec.CapacityPolicy.Zones = []string{"us-central1-a", "us-central1-b"}
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "n2d-standard-4", Architecture: params.Amd64, Zones: []string{"us-central1-a"}},
		{MachineType: "n2-standard-4", Architecture: params.Amd64, Zones: []string{"us-central1-b"}},
	}
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasFirstZone("zones/us-central1-a")), mock.Anything).Return((*compute.Operation)(nil), errors.New("ZONE_RESOURCE_POOL_EXHAUSTED: n2d stockout")).Once()
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasFirstZone("zones/us-central1-b")), mock.Anything).Return((*compute.Operation)(nil), errors.New("RESOURCE_POOL_EXHAUSTED: n2 stockout")).Once()
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Twice()

	_, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "machine_type=n2d-standard-4")
	assert.Contains(t, err.Error(), "n2d stockout")
	assert.Contains(t, err.Error(), "machine_type=n2-standard-4")
	assert.Contains(t, err.Error(), "n2 stockout")
}

func TestAmbiguousCreateErrorDeduplicatesByExactName(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "n2d-standard-4", Architecture: params.Amd64},
		{MachineType: "n2-standard-4", Architecture: params.Amd64},
	}
	regional.On("BulkInsert", ctx, mock.Anything, mock.Anything).Return((*compute.Operation)(nil), errors.New("ZONE_RESOURCE_POOL_EXHAUSTED: context deadline exceeded")).Once()
	created := createdPolicyInstance("us-central1-a")
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return(created, nil).Once()

	result, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.NoError(t, err)
	assert.Equal(t, created, result)
	regional.AssertNumberOfCalls(t, "BulkInsert", 1)
}

func TestNonCapacityErrorsNeverFallback(t *testing.T) {
	errorsByName := map[string]string{
		"authentication":  "UNAUTHENTICATED",
		"permission":      "PERMISSION_DENIED",
		"invalid image":   "source image was not found",
		"invalid disk":    "invalid disk type",
		"invalid machine": "Invalid value for field machineType",
		"invalid network": "Invalid value for networkInterfaces",
		"malformed":       "INVALID_ARGUMENT",
	}
	for name, message := range errorsByName {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			gcpCli, mockClient, regional := policyTestClient(t)
			regional.On("BulkInsert", ctx, mock.Anything, mock.Anything).Return((*compute.Operation)(nil), errors.New(message)).Once()
			mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()

			_, err := gcpCli.createCapacityInstance(ctx, capacityRunnerSpec(), basePolicyInstance())
			require.Error(t, err)
			assert.Contains(t, err.Error(), message)
			regional.AssertNumberOfCalls(t, "BulkInsert", 1)
		})
	}
}

func candidateRanks(candidates []rankedCandidate) []int {
	ranks := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		ranks = append(ranks, candidate.rank)
	}
	return ranks
}

func capacityRunnerSpec() *spec.RunnerSpec {
	return &spec.RunnerSpec{
		CapacityPolicy: &spec.CapacityPolicy{
			Zones: []string{"us-central1-a"},
			Candidates: []spec.CapacityCandidate{
				{MachineType: "n2d-standard-4", Architecture: params.Amd64},
			},
			ProvisioningModels: []string{"SPOT", "STANDARD"},
		},
		Zone: "us-central1-a", NetworkID: "network", SubnetworkID: "subnetwork", NicType: "GVNIC",
		DiskSize: 100, DiskType: "pd-balanced", CustomLabels: map[string]string{"purpose": "runner"},
		ServiceAccounts: []*computepb.ServiceAccount{{Email: proto.String("runner@example.invalid")}},
		BootstrapParams: params.BootstrapInstance{
			Name: "garm-instance", Flavor: "e2-standard-4", Image: "projects/example/global/images/base",
			OSType: params.Linux, OSArch: params.Amd64,
		},
	}
}

func basePolicyInstance() *computepb.Instance {
	return &computepb.Instance{
		Name:                   proto.String("garm-instance"),
		Disks:                  generateBootDisk(100, "projects/example/global/images/base", "", "pd-balanced", map[string]string{"purpose": "runner"}, ""),
		Labels:                 map[string]string{"purpose": "runner"},
		Metadata:               &computepb.Metadata{Items: []*computepb.Items{{Key: proto.String("user-data"), Value: proto.String("#cloud-config")}}},
		NetworkInterfaces:      []*computepb.NetworkInterface{{Network: proto.String("network"), Subnetwork: proto.String("subnetwork"), NicType: proto.String("GVNIC")}},
		ServiceAccounts:        []*computepb.ServiceAccount{{Email: proto.String("runner@example.invalid")}},
		ShieldedInstanceConfig: &computepb.ShieldedInstanceConfig{},
		Tags:                   &computepb.Tags{Items: []string{"runner"}},
	}
}

func createdPolicyInstance(zone string) *computepb.Instance {
	return &computepb.Instance{
		Name: proto.String("garm-instance"), Zone: proto.String("zones/" + zone), Status: proto.String("RUNNING"),
		Labels: map[string]string{"ostype": "linux"}, Disks: []*computepb.AttachedDisk{{Architecture: proto.String("amd64")}},
	}
}

func policyTestClient(t *testing.T) (*GcpCli, *MockGcpClient, *MockRegionalGcpClient) {
	t.Helper()
	mockClient := new(MockGcpClient)
	regional := new(MockRegionalGcpClient)
	previousWaitOp := WaitOp
	WaitOp = func(*compute.Operation, context.Context, ...gax.CallOption) error { return nil }
	t.Cleanup(func() { WaitOp = previousWaitOp })
	return &GcpCli{
		cfg:    &config.Config{ProjectId: "example-project", Zone: "us-central1-a"},
		client: mockClient, regionClient: regional,
	}, mockClient, regional
}

func notFoundError() error {
	err, _ := apierror.FromError(&googleapi.Error{Code: 404, Message: "not found"})
	return err
}

func hasProvisioningModel(model string) func(*computepb.BulkInsertRegionInstanceRequest) bool {
	return func(req *computepb.BulkInsertRegionInstanceRequest) bool {
		scheduling := req.GetBulkInsertInstanceResourceResource().GetInstanceProperties().GetScheduling()
		if model == "STANDARD" {
			return scheduling == nil
		}
		return scheduling.GetProvisioningModel() == model
	}
}

func hasFirstZone(zone string) func(*computepb.BulkInsertRegionInstanceRequest) bool {
	return func(req *computepb.BulkInsertRegionInstanceRequest) bool {
		zones := req.GetBulkInsertInstanceResourceResource().GetLocationPolicy().GetZones()
		return len(zones) > 0 && zones[0].GetZone() == zone
	}
}

func TestRequestIDIsUUID(t *testing.T) {
	requestID, err := newRequestID()
	require.NoError(t, err)
	parts := strings.Split(requestID, "-")
	require.Len(t, parts, 5)
	assert.Equal(t, []int{8, 4, 4, 4, 12}, []int{len(parts[0]), len(parts[1]), len(parts[2]), len(parts[3]), len(parts[4])})
}
