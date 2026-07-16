// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Cloudbase Solutions SRL

package spec

import (
	"encoding/json"
	"testing"

	"github.com/cloudbase/garm-provider-common/params"
	"github.com/stretchr/testify/require"
)

func TestRegionalPlacementValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  *RegionalPlacement
		wantErr string
	}{
		{name: "valid", policy: &RegionalPlacement{Zones: []string{"us-central1-a", "us-central1-b"}}},
		{name: "empty", policy: &RegionalPlacement{}, wantErr: "regional_placement.zones must not be empty"},
		{name: "duplicate", policy: &RegionalPlacement{Zones: []string{"us-central1-a", "us-central1-a"}}, wantErr: `duplicate regional placement zone "us-central1-a"`},
		{name: "cross region", policy: &RegionalPlacement{Zones: []string{"us-central1-a", "us-east1-b"}}, wantErr: "regional placement zones must belong to one region"},
		{name: "malformed", policy: &RegionalPlacement{Zones: []string{"us-central1"}}, wantErr: "expected a zone name with a hyphen-delimited suffix"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.policy.Validate()
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestRegionalPlacementExtraSpecsAreAdditive(t *testing.T) {
	extra, err := newExtraSpecsFromBootstrapData(params.BootstrapInstance{ExtraSpecs: json.RawMessage(`{
		"regional_placement": {"zones": ["us-central1-a", "us-central1-b"]}
	}`)})
	require.NoError(t, err)
	require.Equal(t, "us-central1", extra.RegionalPlacement.Region())
}

func TestRegionalPlacementRejectsUnsupportedExistingOptions(t *testing.T) {
	policy := &RegionalPlacement{Zones: []string{"us-central1-a"}}
	require.ErrorContains(t, (&extraSpecs{RegionalPlacement: policy, DisplayDevice: true}).Validate(), "display_device")
	require.ErrorContains(t, (&extraSpecs{RegionalPlacement: policy, SourceSnapshot: "snapshot"}).Validate(), "source_snapshot")
}
