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
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-gcp/internal/spec"
	"github.com/cloudbase/garm-provider-gcp/internal/util"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestBuildRegionalInsertRequest(t *testing.T) {
	runnerSpec := &spec.RunnerSpec{
		RegionalPlacement: &spec.RegionalPlacement{
			Zones: []string{"us-central1-a", "us-central1-b"},
		},
		BootstrapParams: params.BootstrapInstance{
			Name:   "garm-instance",
			Flavor: "n1-standard-1",
			Image:  "projects/garm-testing/global/images/garm-image",
		},
	}
	instance := &computepb.Instance{
		Name: proto.String("garm-instance"),
		Labels: map[string]string{
			"garmpoolid": "garm-pool",
		},
		Disks: []*computepb.AttachedDisk{
			{
				Boot: proto.Bool(true),
				InitializeParams: &computepb.AttachedDiskInitializeParams{
					SourceImage: proto.String("projects/garm-testing/global/images/garm-image"),
					// generateBootDisk always sets SourceSnapshot, even when empty.
					SourceSnapshot: proto.String(""),
				},
			},
		},
		NetworkInterfaces: []*computepb.NetworkInterface{
			{
				Network: proto.String("my-network"),
			},
		},
	}
	markRegionalInstance(instance)

	req := buildRegionalInsertRequest("my-project", runnerSpec, instance)
	require.Equal(t, "my-project", req.Project)
	require.Equal(t, "us-central1", req.Region)
	require.NotEmpty(t, req.GetRequestId())
	resource := req.BulkInsertInstanceResourceResource
	require.EqualValues(t, 1, resource.GetCount())
	require.EqualValues(t, 1, resource.GetMinCount())
	require.Equal(t, "ANY_SINGLE_ZONE", resource.LocationPolicy.GetTargetShape())
	require.Len(t, resource.LocationPolicy.Zones, 2)
	require.Equal(t, "zones/us-central1-a", resource.LocationPolicy.Zones[0].GetZone())
	require.Equal(t, "n1-standard-1", resource.InstanceProperties.GetMachineType())
	require.Equal(t, "projects/garm-testing/global/images/garm-image", resource.InstanceProperties.Disks[0].InitializeParams.GetSourceImage())
	require.Nil(t, resource.InstanceProperties.Disks[0].InitializeParams.SourceSnapshot)
	require.Equal(t, "true", resource.InstanceProperties.Labels[util.RegionalPlacementLabel])
	require.Contains(t, resource.PerInstanceProperties, "garm-instance")
}

func TestSplitRegionalProviderID(t *testing.T) {
	tests := []struct {
		name         string
		providerID   string
		expectedZone string
		expectedName string
		expectedOk   bool
	}{
		{
			name:         "ZonedProviderID",
			providerID:   "US-CENTRAL1-B/Garm-Instance",
			expectedZone: "us-central1-b",
			expectedName: "garm-instance",
			expectedOk:   true,
		},
		{
			name:       "PlainInstanceName",
			providerID: "garm-instance",
			expectedOk: false,
		},
		{
			name:       "TooManySeparators",
			providerID: "us-central1-b/garm-instance/extra",
			expectedOk: false,
		},
		{
			name:       "MissingName",
			providerID: "us-central1-b/",
			expectedOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zone, name, ok := splitRegionalProviderID(tt.providerID)
			require.Equal(t, tt.expectedOk, ok)
			require.Equal(t, tt.expectedZone, zone)
			require.Equal(t, tt.expectedName, name)
		})
	}
}
