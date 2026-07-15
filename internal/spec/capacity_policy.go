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

package spec

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cloudbase/garm-provider-common/params"
)

const (
	provisioningModelStandard = "STANDARD"
	provisioningModelSpot     = "SPOT"
)

// CapacityPolicy describes regional placement choices in preference order.
type CapacityPolicy struct {
	Zones              []string            `json:"zones" jsonschema:"description=Ordered Compute Engine zones allowed for regional placement."`
	Candidates         []CapacityCandidate `json:"candidates" jsonschema:"description=Ordered machine type candidates. Earlier entries have a lower GCE flexibility rank."`
	ProvisioningModels []string            `json:"provisioning_models" jsonschema:"description=Ordered Compute Engine provisioning models. Supported values are STANDARD and SPOT."`
}

// CapacityCandidate describes one architecture-specific machine selection.
type CapacityCandidate struct {
	MachineType  string        `json:"machine_type" jsonschema:"description=Compute Engine machine type name without a zone or URL."`
	Architecture params.OSArch `json:"architecture" jsonschema:"description=Runner CPU architecture for this candidate. Supported values are amd64 and arm64."`
	Zones        []string      `json:"zones,omitempty" jsonschema:"description=Optional subset of policy zones compatible with this candidate. Empty means every policy zone."`
	Image        string        `json:"image,omitempty" jsonschema:"description=Optional source image override for this candidate."`
	DiskType     string        `json:"disk_type,omitempty" jsonschema:"description=Optional boot disk type override for this candidate."`
	DiskSize     int64         `json:"disk_size,omitempty" jsonschema:"description=Optional boot disk size override in GB. Zero uses the pool disk size."`
}

func (p *CapacityPolicy) Validate() error {
	if p == nil {
		return nil
	}
	if len(p.Zones) == 0 {
		return fmt.Errorf("capacity_policy.zones must not be empty")
	}
	if len(p.Candidates) == 0 {
		return fmt.Errorf("capacity_policy.candidates must not be empty")
	}
	if len(p.ProvisioningModels) == 0 {
		return fmt.Errorf("capacity_policy.provisioning_models must not be empty")
	}

	region := ""
	seenZones := make(map[string]struct{}, len(p.Zones))
	for _, zone := range p.Zones {
		zoneRegion, err := regionFromZone(zone)
		if err != nil {
			return fmt.Errorf("invalid capacity policy zone %q: %w", zone, err)
		}
		if region == "" {
			region = zoneRegion
		} else if zoneRegion != region {
			return fmt.Errorf("capacity policy zones must belong to one region")
		}
		if _, ok := seenZones[zone]; ok {
			return fmt.Errorf("duplicate capacity policy zone %q", zone)
		}
		seenZones[zone] = struct{}{}
	}

	seenModels := make(map[string]struct{}, len(p.ProvisioningModels))
	for _, model := range p.ProvisioningModels {
		if model != provisioningModelStandard && model != provisioningModelSpot {
			return fmt.Errorf("unsupported capacity policy provisioning model %q", model)
		}
		if _, ok := seenModels[model]; ok {
			return fmt.Errorf("duplicate capacity policy provisioning model %q", model)
		}
		seenModels[model] = struct{}{}
	}

	architecture := p.Candidates[0].Architecture
	seenMachineTypes := make(map[string]struct{}, len(p.Candidates))
	for i, candidate := range p.Candidates {
		if candidate.MachineType == "" {
			return fmt.Errorf("capacity policy candidate %d is missing machine_type", i)
		}
		if strings.Contains(candidate.MachineType, "/") {
			return fmt.Errorf("capacity policy candidate %q machine_type must not be a URL", candidate.MachineType)
		}
		if _, ok := seenMachineTypes[candidate.MachineType]; ok {
			return fmt.Errorf("duplicate capacity policy machine type %q", candidate.MachineType)
		}
		seenMachineTypes[candidate.MachineType] = struct{}{}
		if candidate.Architecture != params.Amd64 && candidate.Architecture != params.Arm64 {
			return fmt.Errorf("capacity policy candidate %q has unsupported architecture %q", candidate.MachineType, candidate.Architecture)
		}
		if candidate.Architecture != architecture {
			return fmt.Errorf("capacity policy candidates must use one architecture")
		}
		if candidate.DiskSize < 0 {
			return fmt.Errorf("capacity policy candidate %q disk_size must not be negative", candidate.MachineType)
		}
		seenCandidateZones := make(map[string]struct{}, len(candidate.Zones))
		for _, zone := range candidate.Zones {
			if !slices.Contains(p.Zones, zone) {
				return fmt.Errorf("capacity policy candidate %q zone %q is not allowed by the policy", candidate.MachineType, zone)
			}
			if _, ok := seenCandidateZones[zone]; ok {
				return fmt.Errorf("capacity policy candidate %q has duplicate zone %q", candidate.MachineType, zone)
			}
			seenCandidateZones[zone] = struct{}{}
		}
	}

	return nil
}

func regionFromZone(zone string) (string, error) {
	lastDash := strings.LastIndex(zone, "-")
	if lastDash <= 0 || lastDash == len(zone)-1 || strings.Contains(zone[lastDash+1:], "-") {
		return "", fmt.Errorf("expected a zone name with a hyphen-delimited suffix")
	}
	return zone[:lastDash], nil
}

func (p *CapacityPolicy) Region() string {
	if p == nil || len(p.Zones) == 0 {
		return ""
	}
	region, _ := regionFromZone(p.Zones[0])
	return region
}
