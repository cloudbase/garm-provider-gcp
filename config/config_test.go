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

package config

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		errString error
	}{
		{
			name: "ValidConfig",
			config: &Config{
				Zone:             "europe-west1-d",
				ProjectId:        "my-project",
				NetworkID:        "my-network",
				SubnetworkID:     "my-subnetwork",
				CredentialsFile:  "path/to/credentials.json",
				ExternalIPAccess: true,
			},
			errString: nil,
		},
		{
			name: "MissingRegion",
			config: &Config{
				ProjectId:        "my-project",
				NetworkID:        "my-network",
				SubnetworkID:     "my-subnetwork",
				CredentialsFile:  "path/to/credentials.json",
				ExternalIPAccess: true,
			},
			errString: fmt.Errorf("missing region"),
		},
		{
			name: "MissingProjectID",
			config: &Config{
				Zone:            "europe-west1-d",
				NetworkID:       "my-network",
				SubnetworkID:    "my-subnetwork",
				CredentialsFile: "path/to/credentials.json",
			},
			errString: fmt.Errorf("missing project_id"),
		},
		{
			name: "MissingNetworkID",
			config: &Config{
				Zone:             "europe-west1-d",
				ProjectId:        "my-project",
				SubnetworkID:     "my-subnetwork",
				CredentialsFile:  "path/to/credentials.json",
				ExternalIPAccess: true,
			},
			errString: fmt.Errorf("missing network_id"),
		},
		{
			name: "MissingSubnetworkID",
			config: &Config{
				Zone:             "europe-west1-d",
				ProjectId:        "my-project",
				NetworkID:        "my-network",
				CredentialsFile:  "path/to/credentials.json",
				ExternalIPAccess: true,
			},
			errString: fmt.Errorf("missing subnetwork_id"),
		},
		{
			name: "MissingCredentialsFile",
			config: &Config{
				Zone:             "europe-west1-d",
				ProjectId:        "my-project",
				NetworkID:        "my-network",
				SubnetworkID:     "my-subnetwork",
				ExternalIPAccess: true,
			},
			errString: fmt.Errorf("missing credentials_file"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.errString == nil {
				require.Nil(t, err)
			} else {
				require.Equal(t, tc.errString, err)
			}

		})
	}
}

func TestNewConfig(t *testing.T) {
	mockData := `
	project_id = "garm-testing"
	zone = "europe-west1-d"
	network_id = "projects/garm-testing/global/networks/garm"
	subnetwork_id = "projects/garm-testing/regions/europe-west1/subnetworks/garm"
	credentials_file = "/home/ubuntu/service-account-key.json"
	external_ip_access = true
	`
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	require.NoError(t, err, "Failed to create temporary file")
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(mockData)
	require.NoError(t, err, "Failed to write to temporary file")
	err = tmpFile.Close()
	require.NoError(t, err, "Failed to close temporary file")

	// Use the temporary file path as the argument to NewConfig
	cfg, err := NewConfig(tmpFile.Name())
	require.NoError(t, err, "NewConfig returned an error")

	// Validate the content of the Config object
	require.Equal(t, "garm-testing", cfg.ProjectId, "ProjectId value did not match expected")
	require.Equal(t, "europe-west1-d", cfg.Zone, "Zone value did not match expected")
	require.Equal(t, "projects/garm-testing/global/networks/garm", cfg.NetworkID, "NetworkId value did not match expected")
	require.Equal(t, "projects/garm-testing/regions/europe-west1/subnetworks/garm", cfg.SubnetworkID, "SubnetworkId value did not match expected")
	require.Equal(t, "/home/ubuntu/service-account-key.json", cfg.CredentialsFile, "CredentialsFile value did not match expected")
	require.Equal(t, true, cfg.ExternalIPAccess, "ExternalIpAccess value did not match expected")
}
