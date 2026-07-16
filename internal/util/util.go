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

package util

import (
	"fmt"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/params"
)

const RegionalPlacementLabel string = "garmregionalplacement"

func GetMachineType(zone, flavor string) string {
	machine := fmt.Sprintf("zones/%s/machineTypes/%s", zone, flavor)
	return machine
}

func GetInstanceName(name string) string {
	lowerName := strings.ToLower(name)
	return lowerName
}

func getZoneName(zone string) string {
	parts := strings.Split(strings.TrimSuffix(zone, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func GetRegionalProviderID(instance *computepb.Instance) (string, error) {
	if instance == nil || instance.Labels[RegionalPlacementLabel] != "true" {
		return "", fmt.Errorf("instance is not marked for regional placement")
	}
	zone := getZoneName(instance.GetZone())
	if zone == "" {
		return "", fmt.Errorf("regional instance zone is missing")
	}
	return zone + "/" + GetInstanceName(instance.GetName()), nil
}

func getNameForInstance(instance *computepb.Instance) (string, error) {
	if instance == nil {
		return "", fmt.Errorf("instance is nil")
	}

	var name string
	if instance.Name != nil {
		name = *instance.Name
	}
	if instance.Metadata != nil {
		for _, item := range instance.Metadata.Items {
			if item.Key != nil && *item.Key == "runner_name" {
				if item.Value != nil {
					return *item.Value, nil
				}
			}
		}
	}
	return name, nil
}

func GcpInstanceToParamsInstance(gcpInstance *computepb.Instance) (params.ProviderInstance, error) {
	if gcpInstance == nil {
		return params.ProviderInstance{}, fmt.Errorf("instance ID is nil")
	}
	name, err := getNameForInstance(gcpInstance)
	if err != nil {
		return params.ProviderInstance{}, fmt.Errorf("failed to get instance name: %w", err)
	}
	if gcpInstance.Name == nil {
		return params.ProviderInstance{}, fmt.Errorf("instance name is nil")
	}

	var details params.ProviderInstance
	if gcpInstance.Labels[RegionalPlacementLabel] == "true" {
		providerID, err := GetRegionalProviderID(gcpInstance)
		if err != nil {
			return params.ProviderInstance{}, err
		}
		if len(gcpInstance.GetDisks()) == 0 || gcpInstance.GetDisks()[0].Architecture == nil {
			return params.ProviderInstance{}, fmt.Errorf("regional instance boot disk architecture is missing")
		}
		details = params.ProviderInstance{
			ProviderID: providerID,
			Name:       name,
			OSType:     params.OSType(gcpInstance.Labels["ostype"]),
		}
		switch strings.ToUpper(gcpInstance.Disks[0].GetArchitecture()) {
		case "AMD64", "X86_64":
			details.OSArch = params.Amd64
		case "ARM64":
			details.OSArch = params.Arm64
		default:
			return params.ProviderInstance{}, fmt.Errorf("unsupported regional instance architecture %s", gcpInstance.Disks[0].GetArchitecture())
		}
	} else {
		details = params.ProviderInstance{
			ProviderID: GetInstanceName(*gcpInstance.Name),
			Name:       name,
			OSType:     params.OSType(gcpInstance.Labels["ostype"]),
			OSArch:     params.OSArch(*gcpInstance.Disks[0].Architecture),
		}
	}

	switch gcpInstance.GetStatus() {
	case "RUNNING", "STAGING", "PROVISIONING":
		details.Status = params.InstanceRunning
	case "STOPPING", "TERMINATED", "SUSPENDED":
		details.Status = params.InstanceStopped
	default:
		details.Status = params.InstanceStatusUnknown
	}

	return details, nil
}
