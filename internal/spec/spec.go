// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package spec

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/cloudbase/garm-provider-common/cloudconfig"
	"github.com/cloudbase/garm-provider-common/defaults"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-common/util"
	"github.com/cloudbase/garm-provider-gcp/config"
	utils "github.com/cloudbase/garm-provider-gcp/internal/util"
)

const (
	defaultDiskSizeGB int64  = 127
	defaultNicType    string = "VIRTIO_NET"
	garmPoolID        string = "garmpoolid"
	garmControllerID  string = "garmcontrollerid"
	osType            string = "ostype"
)

func newExtraSpecsFromBootstrapData(data params.BootstrapInstance) (*extraSpecs, error) {
	spec := &extraSpecs{}

	if len(data.ExtraSpecs) > 0 {
		if err := json.Unmarshal(data.ExtraSpecs, spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal extra specs: %w", err)
		}
	}

	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate extra specs: %w", err)
	}

	return spec, nil
}

func (e *extraSpecs) Validate() error {
	if len(e.CustomLabels) > 61 {
		return fmt.Errorf("custom labels cannot exceed 64 items")
	}
	for k, v := range e.CustomLabels {
		if len(k) > 63 {
			return fmt.Errorf("custom label key '%s' exceeds 63 characters", k)
		}
		if len(v) > 63 {
			return fmt.Errorf("custom label value '%s' exceeds 63 characters", v)
		}
		if !utils.IsLower(k) || !utils.IsLower(v) {
			return fmt.Errorf("custom label key '%s' and value '%s' must be lowercase", k, v)
		}
	}
	return nil
}

type extraSpecs struct {
	DiskSize     int64             `json:"disksize,omitempty"`
	NetworkID    string            `json:"network_id,omitempty"`
	SubnetworkID string            `json:"subnetwork_id,omitempty"`
	NicType      string            `json:"nic_type,omitempty"`
	CustomLabels map[string]string `json:"custom_labels,omitempty"`
}

func GetRunnerSpecFromBootstrapParams(cfg *config.Config, data params.BootstrapInstance, controllerID string) (*RunnerSpec, error) {
	tools, err := util.GetTools(data.OSType, data.OSArch, data.Tools)
	if err != nil {
		return nil, fmt.Errorf("failed to get tools: %s", err)
	}

	extraSpecs, err := newExtraSpecsFromBootstrapData(data)
	if err != nil {
		return nil, fmt.Errorf("error loading extra specs: %w", err)
	}

	labels := map[string]string{
		garmPoolID:       data.PoolID,
		garmControllerID: controllerID,
		osType:           string(data.OSType),
	}

	spec := &RunnerSpec{
		Zone:            cfg.Zone,
		Tools:           tools,
		BootstrapParams: data,
		NetworkID:       cfg.NetworkID,
		SubnetworkID:    cfg.SubnetworkID,
		ControllerID:    controllerID,
		NicType:         defaultNicType,
		DiskSize:        defaultDiskSizeGB,
		CustomLabels:    labels,
	}

	spec.MergeExtraSpecs(extraSpecs)

	return spec, nil
}

type RunnerSpec struct {
	Zone            string
	Tools           params.RunnerApplicationDownload
	BootstrapParams params.BootstrapInstance
	NetworkID       string
	SubnetworkID    string
	ControllerID    string
	NicType         string
	DiskSize        int64
	CustomLabels    map[string]string
}

func (r *RunnerSpec) MergeExtraSpecs(extraSpecs *extraSpecs) {
	if extraSpecs.NetworkID != "" {
		r.NetworkID = extraSpecs.NetworkID
	}
	if extraSpecs.SubnetworkID != "" {
		r.SubnetworkID = extraSpecs.SubnetworkID
	}
	if extraSpecs.DiskSize > 0 {
		r.DiskSize = extraSpecs.DiskSize
	}
	if extraSpecs.NicType != "" {
		r.NicType = extraSpecs.NicType
	}
	if len(extraSpecs.CustomLabels) > 0 {
		maps.Copy(r.CustomLabels, extraSpecs.CustomLabels)
	}
}

func (r *RunnerSpec) Validate() error {
	if r.Zone == "" {
		return fmt.Errorf("missing zone")
	}
	if r.NetworkID == "" {
		return fmt.Errorf("missing network id")
	}
	if r.SubnetworkID == "" {
		return fmt.Errorf("missing subnetwork id")
	}
	if r.ControllerID == "" {
		return fmt.Errorf("missing controller id")
	}
	if r.NicType == "" {
		return fmt.Errorf("missing nic type")
	}

	return nil
}

func (r RunnerSpec) ComposeUserData() (string, error) {
	switch r.BootstrapParams.OSType {
	case params.Linux:
		udata, err := cloudconfig.GetRunnerInstallScript(r.BootstrapParams, r.Tools, r.BootstrapParams.Name)
		if err != nil {
			return "", fmt.Errorf("failed to generate userdata: %w", err)
		}

		lines := strings.Split(string(udata), "\n")
		if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
			additionalCommands := []string{
				// Create user 'runner' if it doesn't exist; '|| true' to ignore if user already exists
				"sudo useradd -m " + defaults.DefaultUser + " || true",
				// Create the runner home directory if it doesn't exist
				"sudo mkdir -p /home/" + defaults.DefaultUser,
				// Add user to sudoers
				"sudo usermod -aG sudo " + defaults.DefaultUser,
				// Check curl and tar are installed
				"sudo apt-get update && sudo apt-get install -y curl tar",
			}
			lines = append(lines[:1], append(additionalCommands, lines[1:]...)...)
		}
		modifiedUdata := strings.Join(lines, "\n")
		return modifiedUdata, nil
	case params.Windows:
		udata, err := cloudconfig.GetRunnerInstallScript(r.BootstrapParams, r.Tools, r.BootstrapParams.Name)
		if err != nil {
			return "", fmt.Errorf("failed to generate userdata: %w", err)
		}

		return string(udata), nil
	}
	return "", fmt.Errorf("unsupported OS type for cloud config: %s", r.BootstrapParams.OSType)
}
