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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonSchemaValidation(t *testing.T) {
	tests := []struct {
		name      string
		input     json.RawMessage
		errString string
	}{
		{
			name: "Valid input",
			input: json.RawMessage(`{
				"disksize": 127,
				"network_id": "default",
				"subnet_id": "default",
				"nic_type": "VIRTIO_NET",
				"custom_labels": {
					"example_label": "example_value"
				},
				"network_tags": ["example_tag"],
				"source_snapshot": "snapshot-id",
				"ssh_keys": ["ssh-key", "ssh-key2"],
				"enable_boot_debug": true,
				"runner_install_template": "install-template",
				"extra_context": {
					"key": "value"
				}
			}`),
			errString: "",
		},
		{
			name: "Invalid input - wrong data type",
			input: json.RawMessage(`{
				"disksize": "127"
			}`),
			errString: "schema validation failed: [disksize: Invalid type. Expected: integer, given: string]",
		},
		{
			name: "Invalid input - additional property",
			input: json.RawMessage(`{
				"disksize": 127,
				"additional_property": "value"
			}`),
			errString: "Additional property additional_property is not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := jsonSchemaValidation(tt.input)
			if tt.errString == "" {
				assert.NoError(t, err, "Expected no error, got %v", err)
			} else {
				assert.Error(t, err, "Expected an error")
				// If an error is expected, also check that the error message matches
				if err != nil {
					assert.Contains(t, err.Error(), tt.errString, "Error message does not match")
				}
			}
		})
	}
}

func TestMergeExtraSpecs(t *testing.T) {
	enable_boot_debug := true
	tests := []struct {
		name       string
		extraSpecs *extraSpecs
	}{
		{
			name: "ValidExtraSpecs",
			extraSpecs: &extraSpecs{
				NetworkID:       "projects/garm-testing/global/networks/garm-2",
				SubnetworkID:    "projects/garm-testing/regions/europe-west1/subnetworks/garm",
				DiskSize:        100,
				NicType:         "VIRTIO_NET",
				CustomLabels:    map[string]string{"key1": "value1"},
				NetworkTags:     []string{"tag1", "tag2"},
				SourceSnapshot:  "projects/garm-testing/global/snapshots/garm-snapshot",
				SSHKeys:         []string{"ssh-key1", "ssh-key2"},
				EnableBootDebug: &enable_boot_debug,
			},
		},
		{
			name:       "EmptyExtraSpecs",
			extraSpecs: &extraSpecs{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &RunnerSpec{
				NetworkID:      "default-network",
				SubnetworkID:   "default-subnetwork",
				DiskSize:       50,
				NicType:        "Standard",
				CustomLabels:   map[string]string{"key2": "value2"},
				NetworkTags:    []string{"tag3", "tag4"},
				SourceSnapshot: "default-snapshot",
			}
			spec.MergeExtraSpecs(tt.extraSpecs)
			if tt.extraSpecs.NetworkID != "" {
				if spec.NetworkID != tt.extraSpecs.NetworkID {
					assert.Equal(t, tt.extraSpecs.NetworkID, spec.NetworkID, "expected NetworkID to be %s, got %s", tt.extraSpecs.NetworkID, spec.NetworkID)
				}
			}
			if tt.extraSpecs.SubnetworkID != "" {
				if spec.SubnetworkID != tt.extraSpecs.SubnetworkID {
					assert.Equal(t, tt.extraSpecs.SubnetworkID, spec.SubnetworkID, "expected SubnetworkID to be %s, got %s", tt.extraSpecs.SubnetworkID, spec.SubnetworkID)
				}
			}
			if tt.extraSpecs.DiskSize != 0 {
				if spec.DiskSize != tt.extraSpecs.DiskSize {
					assert.Equal(t, tt.extraSpecs.DiskSize, spec.DiskSize, "expected DiskSize to be %d, got %d", tt.extraSpecs.DiskSize, spec.DiskSize)
				}
			}
			if tt.extraSpecs.NicType != "" {
				if spec.NicType != tt.extraSpecs.NicType {
					assert.Equal(t, tt.extraSpecs.NicType, spec.NicType, "expected NicType to be %s, got %s", tt.extraSpecs.NicType, spec.NicType)
				}
			}
			if len(tt.extraSpecs.CustomLabels) > 0 {
				for k, v := range tt.extraSpecs.CustomLabels {
					if spec.CustomLabels[k] != v {
						t.Errorf("expected CustomLabels[%s] to be %s, got %s", k, v, spec.CustomLabels[k])
					}
				}
			}
			if len(tt.extraSpecs.NetworkTags) > 0 {
				for i, v := range tt.extraSpecs.NetworkTags {
					if spec.NetworkTags[i] != v {
						t.Errorf("expected NetworkTags[%d] to be %s, got %s", i, v, spec.NetworkTags[i])
					}
				}
			}
			if tt.extraSpecs.EnableBootDebug != nil {
				if *tt.extraSpecs.EnableBootDebug != spec.EnableBootDebug {
					assert.Equal(t, *tt.extraSpecs.EnableBootDebug, spec.EnableBootDebug, "expected EnableBootDebug to be %t, got %t", *tt.extraSpecs.EnableBootDebug, spec.EnableBootDebug)
				}
			}

		})
	}
}

func TestRunnerSpec_Validate(t *testing.T) {
	tests := []struct {
		name      string
		spec      *RunnerSpec
		errString error
	}{
		{
			name: "ValidSpec",
			spec: &RunnerSpec{
				Zone:           "europe-west1-d",
				NetworkID:      "projects/garm-testing/global/networks/garm-2",
				SubnetworkID:   "projects/garm-testing/regions/europe-west1/subnetworks/garm",
				ControllerID:   "my-controller",
				NicType:        "VIRTIO_NET",
				DiskSize:       50,
				CustomLabels:   map[string]string{"key1": "value1"},
				NetworkTags:    []string{"tag1", "tag2"},
				SourceSnapshot: "projects/garm-testing/global/snapshots/garm-snapshot",
			},
			errString: nil,
		},
		{
			name: "MissingZone",
			spec: &RunnerSpec{
				NetworkID:      "projects/garm-testing/global/networks/garm-2",
				SubnetworkID:   "projects/garm-testing/regions/europe-west1/subnetworks/garm",
				ControllerID:   "my-controller",
				NicType:        "VIRTIO_NET",
				DiskSize:       50,
				CustomLabels:   map[string]string{"key1": "value1"},
				NetworkTags:    []string{"tag1", "tag2"},
				SourceSnapshot: "projects/garm-testing/global/snapshots/garm-snapshot",
			},
			errString: fmt.Errorf("missing zone"),
		},
		{
			name: "MissingNetworkID",
			spec: &RunnerSpec{
				Zone:           "europe-west1-d",
				SubnetworkID:   "projects/garm-testing/regions/europe-west1/subnetworks/garm",
				ControllerID:   "my-controller",
				NicType:        "VIRTIO_NET",
				DiskSize:       50,
				CustomLabels:   map[string]string{"key1": "value1"},
				NetworkTags:    []string{"tag1", "tag2"},
				SourceSnapshot: "projects/garm-testing/global/snapshots/garm-snapshot",
			},
			errString: fmt.Errorf("missing network id"),
		},
		{
			name: "MissingSubnetworkID",
			spec: &RunnerSpec{
				Zone:           "europe-west1-d",
				NetworkID:      "projects/garm-testing/global/networks/garm-2",
				ControllerID:   "my-controller",
				NicType:        "VIRTIO_NET",
				DiskSize:       50,
				CustomLabels:   map[string]string{"key1": "value1"},
				NetworkTags:    []string{"tag1", "tag2"},
				SourceSnapshot: "projects/garm-testing/global/snapshots/garm-snapshot",
			},
			errString: fmt.Errorf("missing subnetwork id"),
		},
		{
			name: "MissingControllerID",
			spec: &RunnerSpec{
				Zone:           "europe-west1-d",
				NetworkID:      "projects/garm-testing/global/networks/garm-2",
				SubnetworkID:   "projects/garm-testing/regions/europe-west1/subnetworks/garm",
				NicType:        "VIRTIO_NET",
				DiskSize:       50,
				CustomLabels:   map[string]string{"key1": "value1"},
				NetworkTags:    []string{"tag1", "tag2"},
				SourceSnapshot: "projects/garm-testing/global/snapshots/garm-snapshot",
			},
			errString: fmt.Errorf("missing controller id"),
		},
		{
			name: "MissingNicType",
			spec: &RunnerSpec{
				Zone:           "europe-west1-d",
				NetworkID:      "projects/garm-testing/global/networks/garm-2",
				SubnetworkID:   "projects/garm-testing/regions/europe-west1/subnetworks/garm",
				ControllerID:   "my-controller",
				DiskSize:       50,
				CustomLabels:   map[string]string{"key1": "value1"},
				NetworkTags:    []string{"tag1", "tag2"},
				SourceSnapshot: "projects/garm-testing/global/snapshots/garm-snapshot",
			},
			errString: fmt.Errorf("missing nic type"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.errString == nil {
				require.Nil(t, err)
			} else {
				require.Equal(t, tt.errString, err)
			}

		})
	}
}

func TestExtraSpecsValidate(t *testing.T) {
	tests := []struct {
		name    string
		specs   *extraSpecs
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid inputs",
			specs: &extraSpecs{
				CustomLabels: map[string]string{"key1": "value1", "key2": "value2"},
				NetworkTags:  []string{"tag1", "tag2"},
			},
			wantErr: false,
		},
		{
			name: "Too many custom labels",
			specs: &extraSpecs{
				CustomLabels: make(map[string]string),
				NetworkTags:  []string{"tag1", "tag2"},
			},
			wantErr: true,
			errMsg:  "custom labels cannot exceed 61 items",
		},
		{
			name: "Invalid custom label key",
			specs: &extraSpecs{
				CustomLabels: map[string]string{"!invalidKey": "value1"},
				NetworkTags:  []string{"tag1", "tag2"},
			},
			wantErr: true,
			errMsg:  "custom label key '!invalidKey' does not match requirements",
		},
		{
			name: "Invalid custom label value",
			specs: &extraSpecs{
				CustomLabels: map[string]string{"key1": "!invalidValue"},
				NetworkTags:  []string{"tag1", "tag2"},
			},
			wantErr: true,
			errMsg:  "custom label value '!invalidValue' does not match requirements",
		},
		{
			name: "Too many network tags",
			specs: &extraSpecs{
				CustomLabels: map[string]string{"key1": "value1"},
				NetworkTags:  make([]string, 65),
			},
			wantErr: true,
			errMsg:  "network tags cannot exceed 64 items",
		},
		{
			name: "Invalid network tag",
			specs: &extraSpecs{
				CustomLabels: map[string]string{"key1": "value1"},
				NetworkTags:  []string{"!invalidTag"},
			},
			wantErr: true,
			errMsg:  "network tag '!invalidTag' does not match requirements",
		},
	}

	// Generate 62 keys for the "Too many custom labels" test
	for i := 0; i < 62; i++ {
		tests[1].specs.CustomLabels[fmt.Sprintf("key%d", i)] = fmt.Sprintf("value%d", i)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.specs.Validate()
			assert.Equal(t, tt.wantErr, err != nil, "expected error: %v, got: %v", tt.wantErr, err)
		})
	}
}
