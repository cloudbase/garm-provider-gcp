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
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestGcpInstanceToParamsInstance(t *testing.T) {
	tests := []struct {
		name        string
		gcpInstance *computepb.Instance
		expected    params.ProviderInstance
		errString   string
	}{
		{
			name: "ValidGcpInstance",
			gcpInstance: &computepb.Instance{
				Name:   proto.String("garm-instance"),
				Labels: map[string]string{"ostype": "linux"},
				Disks:  []*computepb.AttachedDisk{{Architecture: proto.String("x86_64")}},
				Status: proto.String("RUNNING"),
			},
			expected: params.ProviderInstance{
				ProviderID: "garm-instance",
				Name:       "garm-instance",
				OSType:     "linux",
				OSArch:     "x86_64",
				Status:     "running",
			},
			errString: "",
		},
		{
			name:        "NilGcpInstance",
			gcpInstance: nil,
			expected:    params.ProviderInstance{},
			errString:   "instance ID is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance, err := GcpInstanceToParamsInstance(tt.gcpInstance)
			if tt.errString != "" {
				assert.EqualError(t, err, tt.errString, "expected error %s, got %s", tt.errString, err)
			} else {
				assert.Equal(t, tt.expected, instance, "expected %v, got %v", tt.expected, instance)
			}
		})
	}
}

func TestGetInstanceName(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		expected string
	}{
		{
			name:     "ValidInstanceName",
			instance: "GarmInstance",
			expected: "garminstance",
		},
		{
			name:     "EmptyInstanceName",
			instance: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := GetInstanceName(tt.instance)
			assert.Equal(t, tt.expected, instance, "expected %s, got %s", tt.expected, instance)
		})
	}
}

func TestGetMachineType(t *testing.T) {
	test := struct {
		name     string
		zone     string
		flavor   string
		expected string
	}{
		name:     "ValidMachineType",
		zone:     "us-central1-a",
		flavor:   "n1-standard-1",
		expected: "zones/us-central1-a/machineTypes/n1-standard-1",
	}

	machine := GetMachineType(test.zone, test.flavor)
	assert.Equal(t, test.expected, machine, "expected %s, got %s", test.expected, machine)
}
