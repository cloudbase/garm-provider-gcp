// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Cloudbase Solutions SRL

package client

import (
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-gcp/internal/spec"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestBuildRegionalInsertRequestUsesExistingPoolShape(t *testing.T) {
	runnerSpec := &spec.RunnerSpec{
		RegionalPlacement: &spec.RegionalPlacement{Zones: []string{"us-central1-a", "us-central1-b"}},
		BootstrapParams: params.BootstrapInstance{
			Name: "runner-one", Flavor: "n2d-standard-4", Image: "projects/example/global/images/runner",
		},
	}
	instance := &computepb.Instance{
		Name:   proto.String("runner-one"),
		Labels: map[string]string{"garmpoolid": "pool-one"},
		Disks: []*computepb.AttachedDisk{{
			Boot: proto.Bool(true),
			InitializeParams: &computepb.AttachedDiskInitializeParams{
				SourceImage: proto.String("projects/example/global/images/runner"),
				// The legacy builder emits this empty field. The regional request must not.
				SourceSnapshot: proto.String(""),
			},
		}},
		NetworkInterfaces: []*computepb.NetworkInterface{{Network: proto.String("network")}},
	}
	markRegionalInstance(instance)

	req, err := buildRegionalInsertRequest("project-one", runnerSpec, instance)
	require.NoError(t, err)
	require.Equal(t, "us-central1", req.Region)
	require.NotEmpty(t, req.GetRequestId())
	resource := req.BulkInsertInstanceResourceResource
	require.EqualValues(t, 1, resource.GetCount())
	require.EqualValues(t, 1, resource.GetMinCount())
	require.Equal(t, "ANY_SINGLE_ZONE", resource.LocationPolicy.GetTargetShape())
	require.Len(t, resource.LocationPolicy.Zones, 2)
	require.Equal(t, "n2d-standard-4", resource.InstanceProperties.GetMachineType())
	require.Nil(t, resource.InstanceFlexibilityPolicy)
	require.Equal(t, "projects/example/global/images/runner", resource.InstanceProperties.Disks[0].InitializeParams.GetSourceImage())
	require.Nil(t, resource.InstanceProperties.Disks[0].InitializeParams.SourceSnapshot)
	require.Equal(t, "true", resource.InstanceProperties.Labels[RegionalPlacementLabel])
	require.Contains(t, resource.PerInstanceProperties, "runner-one")
}

func TestSplitRegionalProviderID(t *testing.T) {
	zone, name, ok := splitRegionalProviderID("US-CENTRAL1-B/Runner-One")
	require.True(t, ok)
	require.Equal(t, "us-central1-b", zone)
	require.Equal(t, "runner-one", name)

	_, _, ok = splitRegionalProviderID("legacy-runner")
	require.False(t, ok)
}
