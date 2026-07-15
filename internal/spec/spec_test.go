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

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestJsonSchemaValidation(t *testing.T) {
	tests := []struct {
		name      string
		input     json.RawMessage
		errString string
	}{
		{
			name: "Full specs",
			input: json.RawMessage(`{
				"capacity_policy": {
					"zones": ["us-central1-a", "us-central1-b"],
					"candidates": [
						{"machine_type": "n2-standard-4", "architecture": "amd64"},
						{"machine_type": "c2d-standard-4", "architecture": "amd64", "zones": ["us-central1-b"], "image": "projects/example/global/images/runner", "disk_type": "hyperdisk-balanced", "disk_size": 150}
					],
					"provisioning_models": ["SPOT", "STANDARD"]
				},
				"display_device": true,
				"disksize": 127,
				"disktype": "pd-ssd",
				"network_id": "default",
				"subnetwork_id": "default",
				"nic_type": "VIRTIO_NET",
				"custom_labels": {
					"example_label": "example_value"
				},
				"network_tags": ["example_tag"],
				"service_accounts": [{"email": "email", "scopes": ["scope"]}],
				"service_accounts": [{"email": "email", "scopes": ["scope", "scope2"]}, {"email": "email2", "scopes": ["scope2"]}],
				"source_snapshot": "snapshot-id",
				"ssh_keys": ["ssh-key", "ssh-key2"],
				"enable_boot_debug": true,
				"disable_updates": false,
				"runner_install_template": "IyEvYmluL2Jhc2gKZWNobyBJbnN0YWxsaW5nIHJ1bm5lci4uLg==", "pre_install_scripts": {"setup.sh": "IyEvYmluL2Jhc2gKZWNobyBTZXR1cCBzY3JpcHQuLi4="}, "extra_context": {"key": "value"}
				}`),
			errString: "",
		},
		{
			name: "Specs just with display_device",
			input: json.RawMessage(`{
				"display_device": true
			}`),
			errString: "",
		},
		{
			name: "Capacity policy has a typed candidate list",
			input: json.RawMessage(`{
				"capacity_policy": {
					"zones": ["us-central1-a"],
					"candidates": "n2-standard-4",
					"provisioning_models": ["STANDARD"]
				}
			}`),
			errString: "capacity_policy.candidates: Invalid type. Expected: array, given: string",
		},
		{
			name: "Capacity policy has typed zones",
			input: json.RawMessage(`{
				"capacity_policy": {
					"zones": "us-central1-a",
					"candidates": [{"machine_type": "n2-standard-4", "architecture": "amd64"}],
					"provisioning_models": ["STANDARD"]
				}
			}`),
			errString: "Invalid type. Expected: array, given: string",
		},
		{
			name: "Capacity candidate fields are typed",
			input: json.RawMessage(`{
				"capacity_policy": {
					"zones": ["us-central1-a"],
					"candidates": [{
						"machine_type": "n2-standard-4",
						"architecture": true,
						"zones": "us-central1-a",
						"image": false,
						"disk_type": 1,
						"disk_size": "100"
					}],
					"provisioning_models": ["STANDARD"]
				}
			}`),
			errString: "Invalid type",
		},
		{
			name: "Capacity policy rejects nested additional properties",
			input: json.RawMessage(`{
				"capacity_policy": {
					"zones": ["us-central1-a"],
					"candidates": [{"machine_type": "n2-standard-4", "architecture": "amd64", "priority": 1}],
					"provisioning_models": ["STANDARD"],
					"fallback": true
				}
			}`),
			errString: "Additional property",
		},
		{
			name: "Legacy provisioning fields are typed",
			input: json.RawMessage(`{
				"provisioning_model": false,
				"fallback_to_standard": "true"
			}`),
			errString: "Invalid type",
		},
		{
			name: "Specs just with disksize",
			input: json.RawMessage(`{
				"disksize": 127
			}`),
			errString: "",
		},
		{
			name: "Specs just with disktype",
			input: json.RawMessage(`{
				"disktype": "projects/garm-testing/zones/europe-west1/diskTypes/pd-ssd"
			}`),
			errString: "",
		},
		{
			name: "Specs just with network_id",
			input: json.RawMessage(`{
				"network_id": "default"
			}`),
			errString: "",
		},
		{
			name: "Specs just with subnetwork_id",
			input: json.RawMessage(`{
				"subnetwork_id": "default"
			}`),
			errString: "",
		},
		{
			name: "Specs just with nic_type",
			input: json.RawMessage(`{
				"nic_type": "VIRTIO_NET"
			}`),
			errString: "",
		},
		{
			name: "Specs just with custom_labels",
			input: json.RawMessage(`{
				"custom_labels": {
					"example_label": "example_value"
				}
			}`),
			errString: "",
		},
		{
			name: "Specs just with network_tags",
			input: json.RawMessage(`{
				"network_tags": ["example_tag"]
			}`),
			errString: "",
		},
		{
			name: "Specs just with service_accounts",
			input: json.RawMessage(`{
				"service_accounts": [{"email": "email", "scopes": ["scope"]}]
			}`),
			errString: "",
		},
		{
			name: "Specs just with source_snapshot",
			input: json.RawMessage(`{
				"source_snapshot": "snapshot-id"
			}`),
			errString: "",
		},
		{
			name: "Specs just with ssh_keys",
			input: json.RawMessage(`{
				"ssh_keys": ["ssh-key", "ssh-key2"]
			}`),
			errString: "",
		},
		{
			name: "Specs just with enable_boot_debug",
			input: json.RawMessage(`{
				"enable_boot_debug": true
			}`),
			errString: "",
		},
		{
			name: "Specs just with disable_updates true",
			input: json.RawMessage(`{
				"disable_updates": true
			}`),
			errString: "",
		},
		{
			name: "Specs just with disable_updates false",
			input: json.RawMessage(`{
				"disable_updates": false
			}`),
			errString: "",
		},
		{
			name: "Specs just with runner_install_template",
			input: json.RawMessage(`{
				"runner_install_template": "IyEvYmluL2Jhc2gKZWNobyBJbnN0YWxsaW5nIHJ1bm5lci4uLg=="
			}`),
			errString: "",
		},
		{
			name: "Specs just with pre_install_scripts",
			input: json.RawMessage(`{
				"pre_install_scripts": {
				"setup.sh": "IyEvYmluL2Jhc2gKZWNobyBTZXR1cCBzY3JpcHQuLi4="
				}
			}`),
			errString: "",
		},
		{
			name: "Specs just with extra_context",
			input: json.RawMessage(`{
				"extra_context": {
				"key": "value"
				}
			}`),
			errString: "",
		},
		{
			name: "Invalid input for display_device - wrong data type",
			input: json.RawMessage(`{
				"display_device": "true"
			}`),
			errString: "schema validation failed: [display_device: Invalid type. Expected: boolean, given: string]",
		},
		{
			name: "Invalid input for disksize - wrong data type",
			input: json.RawMessage(`{
				"disksize": "127"
			}`),
			errString: "schema validation failed: [disksize: Invalid type. Expected: integer, given: string]",
		},
		{
			name: "Invalid input for disktype - wrong data type",
			input: json.RawMessage(`{
				"disktype": 127
			}`),
			errString: "schema validation failed: [disktype: Invalid type. Expected: string, given: integer]",
		},
		{
			name: "Invalid input for nic_type - wrong data type",
			input: json.RawMessage(`{
				"nic_type": 127
			}`),
			errString: "schema validation failed: [nic_type: Invalid type. Expected: string, given: integer]",
		},
		{
			name: "Invalid input for custom_labels - wrong data type",
			input: json.RawMessage(`{
				"custom_labels": "example_label"
			}`),
			errString: "schema validation failed: [custom_labels: Invalid type. Expected: object, given: string]",
		},
		{
			name: "Invalid input for network_tags - wrong data type",
			input: json.RawMessage(`{
				"network_tags": "example_tag"
			}`),
			errString: "schema validation failed: [network_tags: Invalid type. Expected: array, given: string]",
		},
		{
			name: "Invalid input for service_accounts - wrong data type",
			input: json.RawMessage(`{
				"service_accounts": "email"
			}`),
			errString: "schema validation failed: [service_accounts: Invalid type. Expected: array, given: string]",
		},
		{
			name: "Invalid input for ssh_keys - wrong data type",
			input: json.RawMessage(`{
				"ssh_keys": "ssh-key"
			}`),
			errString: "schema validation failed: [ssh_keys: Invalid type. Expected: array, given: string]",
		},
		{
			name: "Invalid input for enable_boot_debug - wrong data type",
			input: json.RawMessage(`{
				"enable_boot_debug": "true"
			}`),
			errString: "schema validation failed: [enable_boot_debug: Invalid type. Expected: boolean, given: string]",
		},
		{
			name: "Invalid input for disable_updates - wrong data type",
			input: json.RawMessage(`{
				"disable_updates": "false"
			}`),
			errString: "schema validation failed: [disable_updates: Invalid type. Expected: boolean, given: string]",
		},
		{
			name: "Invalid input for runner_install_template - wrong data type",
			input: json.RawMessage(`{
				"runner_install_template": 127
			}`),
			errString: "schema validation failed: [runner_install_template: Invalid type. Expected: string, given: integer]",
		},
		{
			name: "Invalid input for pre_install_scripts - wrong data type",
			input: json.RawMessage(`{
				"pre_install_scripts": "setup.sh"
			}`),
			errString: "schema validation failed: [pre_install_scripts: Invalid type. Expected: object, given: string]",
		},
		{
			name: "Invalid input for extra_context - wrong data type",
			input: json.RawMessage(`{
				"extra_context": "key"
			}`),
			errString: "schema validation failed: [extra_context: Invalid type. Expected: object, given: string]",
		},
		{
			name: "Invalid input - additional property",
			input: json.RawMessage(`{
				"disksize": 127,
				"additional_property": "value"
			}`),
			errString: "Additional property additional_property is not allowed",
		},
		{
			name: "Invalid json",
			input: json.RawMessage(`{
				"disksize":
			`),
			errString: "failed to validate schema",
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
				NetworkID:     "projects/garm-testing/global/networks/garm-2",
				SubnetworkID:  "projects/garm-testing/regions/europe-west1/subnetworks/garm",
				DisplayDevice: true,
				DiskSize:      100,
				DiskType:      "projects/garm-testing/zones/europe-west1/diskTypes/pd-ssd",
				NicType:       "VIRTIO_NET",
				CustomLabels:  map[string]string{"key1": "value1"},
				NetworkTags:   []string{"tag1", "tag2"},
				ServiceAccounts: []*computepb.ServiceAccount{
					{
						Email:  proto.String("email"),
						Scopes: []string{"scope"},
					},
				},
				SourceSnapshot:  "projects/garm-testing/global/snapshots/garm-snapshot",
				SSHKeys:         []string{"ssh-key1", "ssh-key2"},
				EnableBootDebug: &enable_boot_debug,
				DisableUpdates:  proto.Bool(true),
			},
		},
		{
			name: "ValidExtraSpecsWithDisableUpdatesFalse",
			extraSpecs: &extraSpecs{
				DisableUpdates: proto.Bool(false),
			},
		},
		{
			name: "ValidExtraSpecsWithEnableBootDebugFalse",
			extraSpecs: &extraSpecs{
				EnableBootDebug: proto.Bool(false),
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
				NetworkID:       "default-network",
				DisableUpdates:  false, // Default value
				SubnetworkID:    "default-subnetwork",
				DisplayDevice:   true,
				DiskSize:        50,
				DiskType:        "projects/garm-testing/zones/europe-west1/diskTypes/pd-ssd",
				NicType:         "Standard",
				CustomLabels:    map[string]string{"key2": "value2"},
				NetworkTags:     []string{"tag3", "tag4"},
				SourceSnapshot:  "default-snapshot",
				EnableBootDebug: false, // Default value
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
			if tt.extraSpecs.DiskType != "" {
				if spec.DiskType != tt.extraSpecs.DiskType {
					assert.Equal(t, tt.extraSpecs.DiskType, spec.DiskType, "expected DiskType to be %s, got %s", tt.extraSpecs.DiskType, spec.DiskType)
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
			// Check EnableBootDebug from embedded CloudConfigSpec or direct field if overridden
			expectedEnableBootDebug := spec.EnableBootDebug // Keep default if not set in extraSpecs
			if tt.extraSpecs.EnableBootDebug != nil {
				expectedEnableBootDebug = *tt.extraSpecs.EnableBootDebug
			}
			assert.Equal(t, expectedEnableBootDebug, spec.EnableBootDebug, "expected EnableBootDebug to be %t, got %t", expectedEnableBootDebug, spec.EnableBootDebug)

			expectedDisableUpdates := false // Default for RunnerSpec.DisableUpdates
			if tt.extraSpecs.DisableUpdates != nil {
				expectedDisableUpdates = *tt.extraSpecs.DisableUpdates
			}
			assert.Equal(t, expectedDisableUpdates, spec.DisableUpdates, "expected DisableUpdates to be %t, got %t", expectedDisableUpdates, spec.DisableUpdates)
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
		{
			name: "CapacityPolicyArchitectureMismatch",
			spec: &RunnerSpec{
				Zone: "us-central1-a", NetworkID: "network", SubnetworkID: "subnetwork", ControllerID: "controller", NicType: "GVNIC",
				BootstrapParams: params.BootstrapInstance{OSArch: params.Amd64},
				CapacityPolicy: &CapacityPolicy{
					Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
					Candidates: []CapacityCandidate{{MachineType: "t2a-standard-1", Architecture: params.Arm64}},
				},
			},
			errString: fmt.Errorf("capacity policy architecture \"arm64\" does not match runner architecture \"amd64\""),
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

func TestCapacityPolicyParsingPreservesOrder(t *testing.T) {
	data := params.BootstrapInstance{
		OSType: params.Linux,
		OSArch: params.Amd64,
		ExtraSpecs: json.RawMessage(`{
			"capacity_policy": {
				"zones": ["us-central1-b", "us-central1-a"],
				"candidates": [
					{"machine_type": "n2d-standard-4", "architecture": "amd64"},
					{"machine_type": "n2-standard-4", "architecture": "amd64"}
				],
				"provisioning_models": ["SPOT", "STANDARD"]
			}
		}`),
	}

	extra, err := newExtraSpecsFromBootstrapData(data)
	require.NoError(t, err)
	require.NotNil(t, extra.CapacityPolicy)
	assert.Equal(t, []string{"us-central1-b", "us-central1-a"}, extra.CapacityPolicy.Zones)
	assert.Equal(t, []string{"n2d-standard-4", "n2-standard-4"}, []string{
		extra.CapacityPolicy.Candidates[0].MachineType,
		extra.CapacityPolicy.Candidates[1].MachineType,
	})
	assert.Equal(t, []string{"SPOT", "STANDARD"}, extra.CapacityPolicy.ProvisioningModels)
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
		{
			name: "Invalid provisioning model", specs: &extraSpecs{ProvisioningModel: "PREEMPTIBLE"},
			wantErr: true, errMsg: "provisioning_model must be STANDARD or SPOT",
		},
		{
			name:    "Fallback requires Spot",
			specs:   &extraSpecs{ProvisioningModel: "STANDARD", FallbackToStandard: true},
			wantErr: true, errMsg: "fallback_to_standard requires provisioning_model SPOT",
		},
		{
			name: "Mixed capacity architectures",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{
					{MachineType: "n2-standard-4", Architecture: params.Amd64},
					{MachineType: "t2a-standard-4", Architecture: params.Arm64},
				},
			}},
			wantErr: true, errMsg: "capacity policy candidates must use one architecture",
		},
		{
			name: "Candidate zone outside policy",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64, Zones: []string{"us-central1-b"}}},
			}},
			wantErr: true, errMsg: "zone \"us-central1-b\" is not allowed by the policy",
		},
		{
			name: "Legacy and capacity fields conflict",
			specs: &extraSpecs{
				ProvisioningModel: "SPOT",
				CapacityPolicy: &CapacityPolicy{
					Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
					Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}},
				},
			},
			wantErr: true, errMsg: "capacity_policy cannot be combined",
		},
		{
			name: "Display device is unsupported by regional bulk insert",
			specs: &extraSpecs{
				DisplayDevice: true,
				CapacityPolicy: &CapacityPolicy{
					Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
					Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}},
				},
			},
			wantErr: true, errMsg: "capacity_policy cannot be combined with display_device",
		},
		{
			name: "Empty policy zones",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}}, ProvisioningModels: []string{"STANDARD"},
			}},
			wantErr: true, errMsg: "capacity_policy.zones must not be empty",
		},
		{
			name: "Empty policy candidates",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
			}},
			wantErr: true, errMsg: "capacity_policy.candidates must not be empty",
		},
		{
			name: "Empty policy models",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}},
			}},
			wantErr: true, errMsg: "capacity_policy.provisioning_models must not be empty",
		},
		{
			name: "Cross-region policy zones",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a", "us-east1-b"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}},
			}},
			wantErr: true, errMsg: "capacity policy zones must belong to one region",
		},
		{
			name: "Duplicate policy zone",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a", "us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}},
			}},
			wantErr: true, errMsg: `duplicate capacity policy zone "us-central1-a"`,
		},
		{
			name: "Unsupported policy model",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"PREEMPTIBLE"},
				Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}},
			}},
			wantErr: true, errMsg: `unsupported capacity policy provisioning model "PREEMPTIBLE"`,
		},
		{
			name: "Duplicate policy model",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"SPOT", "SPOT"},
				Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64}},
			}},
			wantErr: true, errMsg: `duplicate capacity policy provisioning model "SPOT"`,
		},
		{
			name: "Missing candidate machine type",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{{Architecture: params.Amd64}},
			}},
			wantErr: true, errMsg: "missing machine_type",
		},
		{
			name: "Candidate machine type URL",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{{MachineType: "zones/us-central1-a/machineTypes/n2-standard-4", Architecture: params.Amd64}},
			}},
			wantErr: true, errMsg: "machine_type must not be a URL",
		},
		{
			name: "Duplicate candidate machine type",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{
					{MachineType: "n2-standard-4", Architecture: params.Amd64},
					{MachineType: "n2-standard-4", Architecture: params.Amd64},
				},
			}},
			wantErr: true, errMsg: `duplicate capacity policy machine type "n2-standard-4"`,
		},
		{
			name: "Unsupported candidate architecture",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.I386}},
			}},
			wantErr: true, errMsg: "unsupported architecture",
		},
		{
			name: "Negative candidate disk size",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64, DiskSize: -1}},
			}},
			wantErr: true, errMsg: "disk_size must not be negative",
		},
		{
			name: "Duplicate candidate zone",
			specs: &extraSpecs{CapacityPolicy: &CapacityPolicy{
				Zones: []string{"us-central1-a"}, ProvisioningModels: []string{"STANDARD"},
				Candidates: []CapacityCandidate{{MachineType: "n2-standard-4", Architecture: params.Amd64, Zones: []string{"us-central1-a", "us-central1-a"}}},
			}},
			wantErr: true, errMsg: `duplicate zone "us-central1-a"`,
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
			if tt.errMsg != "" {
				assert.ErrorContains(t, err, tt.errMsg)
			}
		})
	}
}
