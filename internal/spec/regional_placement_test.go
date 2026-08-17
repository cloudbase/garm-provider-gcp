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

package spec

import (
	"encoding/json"
	"testing"

	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-gcp/config"
	"github.com/stretchr/testify/require"
)

func TestRegionalPlacementValidate(t *testing.T) {
	tests := []struct {
		name      string
		placement *RegionalPlacement
		errString string
	}{
		{
			name:      "ValidZones",
			placement: &RegionalPlacement{Zones: []string{"us-central1-a", "us-central1-b"}},
			errString: "",
		},
		{
			name:      "MissingZones",
			placement: &RegionalPlacement{},
			errString: "regional_placement.zones must not be empty",
		},
		{
			name:      "DuplicateZone",
			placement: &RegionalPlacement{Zones: []string{"us-central1-a", "us-central1-a"}},
			errString: "duplicate regional placement zone 'us-central1-a'",
		},
		{
			name:      "ZonesAcrossRegions",
			placement: &RegionalPlacement{Zones: []string{"us-central1-a", "us-east1-b"}},
			errString: "regional placement zones must belong to one region",
		},
		{
			name:      "MalformedZone",
			placement: &RegionalPlacement{Zones: []string{"us-central1"}},
			errString: "expected a zone name with a hyphen-delimited suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.placement.Validate()
			if tt.errString == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.errString)
			}
		})
	}
}

func TestRegionalPlacementRegion(t *testing.T) {
	var placement *RegionalPlacement
	require.Equal(t, "", placement.Region())

	placement = &RegionalPlacement{Zones: []string{"us-central1-a", "us-central1-b"}}
	require.Equal(t, "us-central1", placement.Region())
}

func TestRegionalPlacementRequiresProviderOptIn(t *testing.T) {
	DefaultToolFetch = func(osType params.OSType, osArch params.OSArch, tools []params.RunnerApplicationDownload) (params.RunnerApplicationDownload, error) {
		return params.RunnerApplicationDownload{}, nil
	}
	cfg := &config.Config{
		Zone:         "europe-west1-d",
		ProjectId:    "my-project",
		NetworkID:    "my-network",
		SubnetworkID: "my-subnetwork",
	}
	data := params.BootstrapInstance{
		Name:       "garm-instance",
		ExtraSpecs: json.RawMessage(`{"regional_placement": {"zones": ["us-central1-a", "us-central1-b"]}}`),
	}

	_, err := GetRunnerSpecFromBootstrapParams(cfg, data, "my-controller")
	require.ErrorContains(t, err, "requires enable_regional_placement")

	cfg.EnableRegionalPlacement = true
	spec, err := GetRunnerSpecFromBootstrapParams(cfg, data, "my-controller")
	require.NoError(t, err)
	require.Equal(t, "us-central1", spec.RegionalPlacement.Region())
}
