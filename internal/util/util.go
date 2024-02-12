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

func GcpInstanceToParamsInstance(gcpInstance *computepb.Instance) (params.ProviderInstance, error) {
	if gcpInstance == nil {
		return params.ProviderInstance{}, fmt.Errorf("instance ID is nil")
	}
	details := params.ProviderInstance{
		ProviderID: GetInstanceName(*gcpInstance.Name),
		Name:       GetInstanceName(*gcpInstance.Name),
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
