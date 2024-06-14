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

func GetMachineType(zone, flavor string) string {
	machine := fmt.Sprintf("zones/%s/machineTypes/%s", zone, flavor)
	return machine
}

func GetInstanceName(name string) string {
	lowerName := strings.ToLower(name)
	return lowerName
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

	details := params.ProviderInstance{
		ProviderID: GetInstanceName(*gcpInstance.Name),
		Name:       name,
		OSType:     params.OSType(gcpInstance.Labels["ostype"]),
		OSArch:     params.OSArch(*gcpInstance.Disks[0].Architecture),
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
