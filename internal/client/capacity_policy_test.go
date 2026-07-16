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
	"encoding/json"
	"errors"
	"fmt"
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
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestBuildPlacementAttemptsOrderingAndZoneCompatibility(t *testing.T) {
	policy := &spec.CapacityPolicy{
		Zones: []string{"example-region-a", "example-region-b", "example-region-c"},
		Candidates: []spec.CapacityCandidate{
			{MachineType: "t2a-standard-2", Architecture: params.Arm64, Zones: []string{"example-region-a", "example-region-b"}},
			{MachineType: "c4a-standard-2", Architecture: params.Arm64},
			{MachineType: "c4a-highcpu-4", Architecture: params.Arm64},
		},
		ProvisioningModels: []string{"SPOT", "STANDARD"},
	}

	attempts := buildPlacementAttempts(policy)
	require.Len(t, attempts, 4)
	assert.Equal(t, "SPOT", attempts[0].model)
	assert.Equal(t, []string{"example-region-a", "example-region-b"}, attempts[0].zones)
	assert.Equal(t, []int{0}, candidateRanks(attempts[0].candidates))
	assert.Equal(t, []string{"example-region-a", "example-region-b", "example-region-c"}, attempts[1].zones)
	assert.Equal(t, []int{1, 2}, candidateRanks(attempts[1].candidates))
	assert.Equal(t, "STANDARD", attempts[2].model)
	assert.Equal(t, []int{0}, candidateRanks(attempts[2].candidates))
	assert.Equal(t, []int{1, 2}, candidateRanks(attempts[3].candidates))
}

func TestBuildPlacementAttemptsTreatsCandidateZonesAsASet(t *testing.T) {
	policy := &spec.CapacityPolicy{
		Zones: []string{"example-region-a", "example-region-b", "example-region-c"},
		Candidates: []spec.CapacityCandidate{
			{MachineType: "n2d-standard-4", Architecture: params.Amd64, Zones: []string{"example-region-b", "example-region-a"}},
			{MachineType: "n2-standard-4", Architecture: params.Amd64, Zones: []string{"example-region-a", "example-region-b"}},
		},
		ProvisioningModels: []string{"STANDARD"},
	}

	attempts := buildPlacementAttempts(policy)
	require.Len(t, attempts, 1)
	assert.Equal(t, []string{"example-region-a", "example-region-b"}, attempts[0].zones)
	assert.Equal(t, []int{0, 1}, candidateRanks(attempts[0].candidates))
}

func TestBuildBulkInsertRequest(t *testing.T) {
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Zones = []string{"example-region-a", "example-region-b"}
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
	assert.Equal(t, "example-region", req.Region)
	assert.NotEmpty(t, req.GetRequestId())
	resource := req.GetBulkInsertInstanceResourceResource()
	assert.EqualValues(t, 1, resource.GetCount())
	assert.EqualValues(t, 1, resource.GetMinCount())
	assert.Equal(t, "ANY_SINGLE_ZONE", resource.GetLocationPolicy().GetTargetShape())
	assert.Equal(t, []string{"zones/example-region-a", "zones/example-region-b"}, []string{
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

func TestBuildBulkInsertRequestMatchesSDKWireShape(t *testing.T) {
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{{
		MachineType: "n2d-standard-4", Architecture: params.Amd64,
		Image: "projects/example/global/images/override", DiskType: "hyperdisk-balanced", DiskSize: 150,
	}}
	inst := basePolicyInstance()
	req, err := buildBulkInsertRequest("example-project", runnerSpec, inst, "SPOT", runnerSpec.CapacityPolicy.Zones, []rankedCandidate{{
		candidate: runnerSpec.CapacityPolicy.Candidates[0], rank: 0,
	}})
	require.NoError(t, err)

	wire, err := (protojson.MarshalOptions{AllowPartial: true}).Marshal(req.GetBulkInsertInstanceResourceResource())
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(wire, &body))
	assert.Equal(t, "1", body["count"])
	assert.Equal(t, "1", body["minCount"])
	assert.Contains(t, body["perInstanceProperties"], "garm-instance")

	properties := body["instanceProperties"].(map[string]any)
	assert.NotContains(t, properties, "disks")
	assert.Equal(t, "SPOT", properties["scheduling"].(map[string]any)["provisioningModel"])
	assert.Equal(t, "GVNIC", properties["networkInterfaces"].([]any)[0].(map[string]any)["nicType"])
	assert.Equal(t, "runner@example.invalid", properties["serviceAccounts"].([]any)[0].(map[string]any)["email"])

	policy := body["instanceFlexibilityPolicy"].(map[string]any)
	selection := policy["instanceSelections"].(map[string]any)["selection-000"].(map[string]any)
	assert.Equal(t, "0", selection["rank"])
	assert.Equal(t, []any{"n2d-standard-4"}, selection["machineTypes"])
	initializeParams := selection["disks"].([]any)[0].(map[string]any)["initializeParams"].(map[string]any)
	assert.Equal(t, "projects/example/global/images/override", initializeParams["sourceImage"])
	assert.Equal(t, "hyperdisk-balanced", initializeParams["diskType"])
	assert.Equal(t, "150", initializeParams["diskSizeGb"])
	assert.Equal(t, "X86_64", initializeParams["architecture"])
	assert.NotContains(t, initializeParams, "sourceSnapshot")
}

func TestClassifyPlacementError(t *testing.T) {
	structuredQuota, _ := apierror.FromError(&googleapi.Error{
		Code: 403, Message: "request failed", Errors: []googleapi.ErrorItem{{Reason: "quotaExceeded"}},
	})
	structuredCapacity, _ := apierror.FromError(&googleapi.Error{
		Code: 503, Message: "request failed", Errors: []googleapi.ErrorItem{{Reason: "resourcePoolExhausted"}},
	})
	structuredInvalid, _ := apierror.FromError(&googleapi.Error{
		Code: 400, Message: "invalid image resource_pool_exhausted", Errors: []googleapi.ErrorItem{{Reason: "invalidArgument"}},
	})
	tests := []struct {
		name  string
		err   error
		class placementErrorClass
	}{
		{name: "zonal stockout", err: errors.New("ZONE_RESOURCE_POOL_EXHAUSTED"), class: placementErrorCapacity},
		{name: "resource not ready", err: errors.New("resourceNotReady"), class: placementErrorCapacity},
		{name: "quota", err: errors.New("QUOTA_EXCEEDED"), class: placementErrorQuota},
		{name: "quota message", err: errors.New("Quota 'N2_CPUS' exceeded"), class: placementErrorQuota},
		{name: "structured quota", err: structuredQuota, class: placementErrorQuota},
		{name: "structured capacity", err: structuredCapacity, class: placementErrorCapacity},
		{name: "structured invalid overrides message", err: structuredInvalid, class: placementErrorTerminal},
		{name: "ambiguous capacity timeout", err: fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED: %w", context.DeadlineExceeded), class: placementErrorTerminal},
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

func TestRegionalWaitOpFailuresUsePlacementClassification(t *testing.T) {
	tests := []struct {
		name       string
		waitErr    error
		models     []string
		candidates []spec.CapacityCandidate
		wantCalls  int
		wantError  bool
	}{
		{
			name: "capacity advances provisioning model", waitErr: errors.New("ZONE_RESOURCE_POOL_EXHAUSTED"),
			models:     []string{"SPOT", "STANDARD"},
			candidates: []spec.CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}},
			wantCalls:  2,
		},
		{
			name: "quota advances ranked candidate", waitErr: errors.New("QUOTA_EXCEEDED: N2D_CPUS"),
			models: []string{"STANDARD"},
			candidates: []spec.CapacityCandidate{
				{MachineType: "n2d-standard-4", Architecture: params.Amd64},
				{MachineType: "n2-standard-4", Architecture: params.Amd64},
			},
			wantCalls: 2,
		},
		{
			name: "terminal stops placement", waitErr: errors.New("PERMISSION_DENIED"),
			models:     []string{"SPOT", "STANDARD"},
			candidates: []spec.CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}},
			wantCalls:  1, wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			gcpCli, mockClient, regional := policyTestClient(t)
			runnerSpec := capacityRunnerSpec()
			runnerSpec.CapacityPolicy.ProvisioningModels = test.models
			runnerSpec.CapacityPolicy.Candidates = test.candidates
			expectNoExistingPolicyInstance(mockClient, ctx, runnerSpec.CapacityPolicy.Zones...)

			waitCalls := 0
			WaitOp = func(*compute.Operation, context.Context, ...gax.CallOption) error {
				waitCalls++
				if waitCalls == 1 {
					return test.waitErr
				}
				return nil
			}
			regional.On("BulkInsert", ctx, mock.Anything, mock.Anything).Return(&compute.Operation{}, nil).Times(test.wantCalls)
			mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()
			if !test.wantError {
				mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return(createdPolicyInstance("example-region-a"), nil).Once()
			}

			result, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
			if test.wantError {
				require.ErrorIs(t, err, test.waitErr)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "zones/example-region-a", result.GetZone())
			}
			assert.Equal(t, test.wantCalls, waitCalls)
			regional.AssertNumberOfCalls(t, "BulkInsert", test.wantCalls)
		})
	}
}

func TestSuccessfulBulkInsertRequiresFollowUpLookup(t *testing.T) {
	lookupErr := errors.New("lookup unavailable")
	tests := []struct {
		name      string
		lookupErr error
		wantText  string
	}{
		{name: "lookup error", lookupErr: lookupErr, wantText: "failed to resolve created instance"},
		{name: "instance missing", lookupErr: notFoundError(), wantText: "regional bulk insert completed without instance"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			gcpCli, mockClient, regional := policyTestClient(t)
			runnerSpec := capacityRunnerSpec()
			runnerSpec.CapacityPolicy.ProvisioningModels = []string{"STANDARD"}
			expectNoExistingPolicyInstance(mockClient, ctx, runnerSpec.CapacityPolicy.Zones...)
			regional.On("BulkInsert", ctx, mock.Anything, mock.Anything).Return(&compute.Operation{}, nil).Once()
			mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), test.lookupErr).Once()

			_, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
			require.ErrorContains(t, err, test.wantText)
			if test.lookupErr == lookupErr {
				require.ErrorIs(t, err, lookupErr)
			}
			regional.AssertNumberOfCalls(t, "BulkInsert", 1)
		})
	}
}

func TestCreateErrorReconciliationPreservesBothErrors(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	createErr := errors.New("PERMISSION_DENIED")
	lookupErr := errors.New("lookup unavailable")
	expectNoExistingPolicyInstance(mockClient, ctx, capacityRunnerSpec().CapacityPolicy.Zones...)
	regional.On("BulkInsert", ctx, mock.Anything, mock.Anything).Return((*compute.Operation)(nil), createErr).Once()
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), lookupErr).Once()

	_, err := gcpCli.createCapacityInstance(ctx, capacityRunnerSpec(), basePolicyInstance())
	require.ErrorIs(t, err, createErr)
	require.ErrorIs(t, err, lookupErr)
	assert.Contains(t, err.Error(), "failed to reconcile create error")
	assert.Contains(t, err.Error(), "lookup failed")
}

func TestQuotaAdvancesCandidateWithDistinctLog(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "n2d-standard-4", Architecture: params.Amd64},
		{MachineType: "n2-standard-4", Architecture: params.Amd64},
	}
	expectNoExistingPolicyInstance(mockClient, ctx, runnerSpec.CapacityPolicy.Zones...)
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
	created := createdPolicyInstance("example-region-a")
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
	expectNoExistingPolicyInstance(mockClient, ctx, runnerSpec.CapacityPolicy.Zones...)
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasProvisioningModel("SPOT")), mock.Anything).Return((*compute.Operation)(nil), errors.New("ZONE_RESOURCE_POOL_EXHAUSTED")).Once()
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasProvisioningModel("STANDARD")), mock.Anything).Return(&compute.Operation{}, nil).Once()
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()
	created := createdPolicyInstance("example-region-a")
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return(created, nil).Once()

	result, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.NoError(t, err)
	assert.Equal(t, "zones/example-region-a", result.GetZone())
	regional.AssertNumberOfCalls(t, "BulkInsert", 2)
}

func TestCapacityErrorAdvancesZoneCompatibleCandidate(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.BootstrapParams.OSArch = params.Arm64
	runnerSpec.CapacityPolicy.ProvisioningModels = []string{"STANDARD"}
	runnerSpec.CapacityPolicy.Zones = []string{"example-region-a", "example-region-b"}
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "t2a-standard-2", Architecture: params.Arm64, Zones: []string{"example-region-a"}},
		{MachineType: "c4a-standard-2", Architecture: params.Arm64},
	}
	expectNoExistingPolicyInstance(mockClient, ctx, runnerSpec.CapacityPolicy.Zones...)
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasFirstZone("zones/example-region-a")), mock.Anything).Return((*compute.Operation)(nil), errors.New("ZONE_RESOURCE_POOL_EXHAUSTED: t2a stockout")).Once()
	regional.On("BulkInsert", ctx, mock.MatchedBy(func(req *computepb.BulkInsertRegionInstanceRequest) bool {
		zones := req.GetBulkInsertInstanceResourceResource().GetLocationPolicy().GetZones()
		return len(zones) == 2 && zones[1].GetZone() == "zones/example-region-b"
	}), mock.Anything).Return(&compute.Operation{}, nil).Once()
	mockClient.On("Get", ctx, mock.MatchedBy(func(req *computepb.GetInstanceRequest) bool {
		return req.Zone == "example-region-a"
	}), mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Twice()
	created := createdPolicyInstance("example-region-b")
	mockClient.On("Get", ctx, mock.MatchedBy(func(req *computepb.GetInstanceRequest) bool {
		return req.Zone == "example-region-b"
	}), mock.Anything).Return(created, nil).Once()

	result, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.NoError(t, err)
	assert.Equal(t, "zones/example-region-b", result.GetZone())
	regional.AssertNumberOfCalls(t, "BulkInsert", 2)
}

func TestQuotaDoesNotAdvanceProvisioningModel(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}}
	expectNoExistingPolicyInstance(mockClient, ctx, runnerSpec.CapacityPolicy.Zones...)
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
	runnerSpec.CapacityPolicy.Zones = []string{"example-region-a", "example-region-b"}
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "n2d-standard-4", Architecture: params.Amd64, Zones: []string{"example-region-a"}},
		{MachineType: "n2-standard-4", Architecture: params.Amd64, Zones: []string{"example-region-b"}},
	}
	expectNoExistingPolicyInstance(mockClient, ctx, runnerSpec.CapacityPolicy.Zones...)
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasFirstZone("zones/example-region-a")), mock.Anything).Return((*compute.Operation)(nil), errors.New("ZONE_RESOURCE_POOL_EXHAUSTED: n2d stockout")).Once()
	regional.On("BulkInsert", ctx, mock.MatchedBy(hasFirstZone("zones/example-region-b")), mock.Anything).Return((*compute.Operation)(nil), errors.New("RESOURCE_POOL_EXHAUSTED: n2 stockout")).Once()
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
	runnerSpec.CapacityPolicy.Zones = []string{"example-region-a", "example-region-b"}
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "n2d-standard-4", Architecture: params.Amd64},
		{MachineType: "n2-standard-4", Architecture: params.Amd64},
	}
	expectNoExistingPolicyInstance(mockClient, ctx, runnerSpec.CapacityPolicy.Zones...)
	regional.On("BulkInsert", ctx, mock.Anything, mock.Anything).Return((*compute.Operation)(nil), fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED: %w", context.DeadlineExceeded)).Once()
	mockClient.On("Get", mock.Anything, &computepb.GetInstanceRequest{
		Project: "example-project", Zone: "example-region-a", Instance: "garm-instance",
	}, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()
	created := createdPolicyInstance("example-region-b")
	mockClient.On("Get", mock.Anything, &computepb.GetInstanceRequest{
		Project: "example-project", Zone: "example-region-b", Instance: "garm-instance",
	}, mock.Anything).Return(created, nil).Once()

	result, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.NoError(t, err)
	assert.Equal(t, created, result)
	regional.AssertNumberOfCalls(t, "BulkInsert", 1)
}

func TestAmbiguousCreateErrorWithoutInstanceNeverAdvances(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	runnerSpec := capacityRunnerSpec()
	runnerSpec.CapacityPolicy.Candidates = []spec.CapacityCandidate{
		{MachineType: "n2d-standard-4", Architecture: params.Amd64},
		{MachineType: "n2-standard-4", Architecture: params.Amd64},
	}
	expectNoExistingPolicyInstance(mockClient, ctx, runnerSpec.CapacityPolicy.Zones...)
	ambiguousErr := fmt.Errorf("ZONE_RESOURCE_POOL_EXHAUSTED: %w", context.DeadlineExceeded)
	regional.On("BulkInsert", ctx, mock.Anything, mock.Anything).Return((*compute.Operation)(nil), ambiguousErr).Once()
	mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()

	_, err := gcpCli.createCapacityInstance(ctx, runnerSpec, basePolicyInstance())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "ZONE_RESOURCE_POOL_EXHAUSTED")
	regional.AssertNumberOfCalls(t, "BulkInsert", 1)
}

func TestAmbiguousCreateReconciliationDetachesCanceledContext(t *testing.T) {
	type contextKey string
	const traceKey contextKey = "trace"

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), traceKey, "preserved"))
	gcpCli, mockClient, regional := policyTestClient(t)
	expectNoExistingPolicyInstance(mockClient, ctx, capacityRunnerSpec().CapacityPolicy.Zones...)
	regional.On("BulkInsert", ctx, mock.Anything, mock.Anything).Run(func(mock.Arguments) { cancel() }).Return((*compute.Operation)(nil), context.Canceled).Once()
	created := createdPolicyInstance("example-region-a")
	mockClient.On("Get", mock.MatchedBy(func(lookupCtx context.Context) bool {
		return lookupCtx.Err() == nil && lookupCtx.Value(traceKey) == "preserved"
	}), &computepb.GetInstanceRequest{
		Project: "example-project", Zone: "example-region-a", Instance: "garm-instance",
	}, mock.Anything).Return(created, nil).Once()

	result, err := gcpCli.createCapacityInstance(ctx, capacityRunnerSpec(), basePolicyInstance())
	require.NoError(t, err)
	assert.Equal(t, created, result)
	regional.AssertNumberOfCalls(t, "BulkInsert", 1)
}

func TestExistingCapacityInstanceSkipsBulkInsert(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	existing := matchingPolicyInstance("example-region-a")
	mockClient.On("Get", ctx, &computepb.GetInstanceRequest{
		Project: "example-project", Zone: "example-region-a", Instance: "garm-instance",
	}, mock.Anything).Return(existing, nil).Once()

	result, err := gcpCli.createCapacityInstance(ctx, capacityRunnerSpec(), basePolicyInstance())
	require.NoError(t, err)
	assert.Equal(t, existing, result)
	regional.AssertNotCalled(t, "BulkInsert", mock.Anything, mock.Anything, mock.Anything)
}

func TestExistingCapacityInstanceWithWrongIdentityFailsClosed(t *testing.T) {
	ctx := context.Background()
	gcpCli, mockClient, regional := policyTestClient(t)
	unrelated := createdPolicyInstance("example-region-a")
	mockClient.On("Get", ctx, mock.Anything, mock.Anything).Return(unrelated, nil).Once()

	_, err := gcpCli.createCapacityInstance(ctx, capacityRunnerSpec(), basePolicyInstance())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capacity policy marker is missing")
	regional.AssertNotCalled(t, "BulkInsert", mock.Anything, mock.Anything, mock.Anything)
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
			expectNoExistingPolicyInstance(mockClient, ctx, capacityRunnerSpec().CapacityPolicy.Zones...)
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
			Zones: []string{"example-region-a"},
			Candidates: []spec.CapacityCandidate{
				{MachineType: "n2d-standard-4", Architecture: params.Amd64},
			},
			ProvisioningModels: []string{"SPOT", "STANDARD"},
		},
		Zone: "example-region-a", NetworkID: "network", SubnetworkID: "subnetwork", NicType: "GVNIC",
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

func matchingPolicyInstance(zone string) *computepb.Instance {
	instance := createdPolicyInstance(zone)
	instance.Labels["purpose"] = "runner"
	instance.Metadata = &computepb.Metadata{Items: []*computepb.Items{{
		Key: proto.String(util.CapacityPolicyMetadataKey), Value: proto.String("true"),
	}}}
	return instance
}

func expectNoExistingPolicyInstance(mockClient *MockGcpClient, ctx context.Context, zones ...string) {
	for _, zone := range zones {
		mockClient.On("Get", ctx, &computepb.GetInstanceRequest{
			Project: "example-project", Zone: zone, Instance: "garm-instance",
		}, mock.Anything).Return((*computepb.Instance)(nil), notFoundError()).Once()
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
		cfg:    &config.Config{ProjectId: "example-project", Zone: "example-region-a"},
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
