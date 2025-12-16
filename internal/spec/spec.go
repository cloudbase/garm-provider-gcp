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
	"regexp"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/cloudconfig"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-common/util"
	"github.com/cloudbase/garm-provider-gcp/config"
	"github.com/invopop/jsonschema"
	"github.com/xeipuuv/gojsonschema"
)

const (
	defaultDiskSizeGB     int64  = 127
	defaultNicType        string = "VIRTIO_NET"
	garmPoolID            string = "garmpoolid"
	garmControllerID      string = "garmcontrollerid"
	osType                string = "ostype"
	customLabelKeyRegex   string = "^\\p{Ll}[\\p{Ll}0-9_-]{0,62}$"
	customLabelValueRegex string = "^[\\p{Ll}0-9_-]{0,63}$"
	networkTagRegex       string = "^[a-z][a-z0-9-]{0,61}[a-z0-9]$"
)

type ToolFetchFunc func(osType params.OSType, osArch params.OSArch, tools []params.RunnerApplicationDownload) (params.RunnerApplicationDownload, error)

var DefaultToolFetch ToolFetchFunc = util.GetTools
var DefaultCloudConfigFunc = cloudconfig.GetCloudConfig
var DefaultRunnerInstallScriptFunc = cloudconfig.GetRunnerInstallScript

func generateJSONSchema() *jsonschema.Schema {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
	}
	// Reflect the extraSpecs struct
	schema := reflector.Reflect(extraSpecs{})

	return schema
}

func jsonSchemaValidation(schema json.RawMessage) error {
	jsonSchema := generateJSONSchema()
	schemaLoader := gojsonschema.NewGoLoader(jsonSchema)
	extraSpecsLoader := gojsonschema.NewBytesLoader(schema)
	result, err := gojsonschema.Validate(schemaLoader, extraSpecsLoader)
	if err != nil {
		return fmt.Errorf("failed to validate schema: %w", err)
	}
	if !result.Valid() {
		return fmt.Errorf("schema validation failed: %s", result.Errors())
	}
	return nil
}

func newExtraSpecsFromBootstrapData(data params.BootstrapInstance) (*extraSpecs, error) {
	spec := &extraSpecs{}

	if err := jsonSchemaValidation(data.ExtraSpecs); err != nil {
		return nil, fmt.Errorf("failed to validate extra specs: %w", err)
	}

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
		return fmt.Errorf("custom labels cannot exceed 61 items")
	}
	keyRegex, err := regexp.Compile(customLabelKeyRegex)
	if err != nil {
		return fmt.Errorf("invalid key regex pattern: %w", err)

	}
	valueRegex, err := regexp.Compile(customLabelValueRegex)
	if err != nil {
		return fmt.Errorf("invalid value regex pattern: %w", err)
	}
	for key, value := range e.CustomLabels {
		if !keyRegex.MatchString(key) {
			return fmt.Errorf("custom label key '%s' does not match requirements", key)
		}
		if !valueRegex.MatchString(value) {
			return fmt.Errorf("custom label value '%s' does not match requirements", value)
		}
	}
	if len(e.NetworkTags) > 64 {
		return fmt.Errorf("network tags cannot exceed 64 items")
	}
	tagRegex, err := regexp.Compile(networkTagRegex)
	if err != nil {
		return fmt.Errorf("invalid tag regex pattern: %w", err)
	}
	for _, tag := range e.NetworkTags {
		if !tagRegex.MatchString(tag) {
			return fmt.Errorf("network tag '%s' does not match requirements", tag)
		}
	}
	return nil
}

type extraSpecs struct {
	DiskSize        int64                       `json:"disksize,omitempty" jsonschema:"description=The size of the root disk in GB. Default is 127 GB."`
	DiskType        string                      `json:"disktype,omitempty" jsonschema:"description=The type of the disk. Default is pd-standard."`
	DisplayDevice   bool                        `json:"display_device,omitempty" jsonschema:"description=Enable the display device on the VM."`
	NetworkID       string                      `json:"network_id,omitempty" jsonschema:"description=The name of the network attached to the instance."`
	SubnetworkID    string                      `json:"subnetwork_id,omitempty" jsonschema:"description=The name of the subnetwork attached to the instance."`
	NicType         string                      `json:"nic_type,omitempty" jsonschema:"description=The type of the network interface card. Default is VIRTIO_NET."`
	CustomLabels    map[string]string           `json:"custom_labels,omitempty" jsonschema:"description=Custom labels to apply to the instance. Each label is a key-value pair where both key and value are strings."`
	NetworkTags     []string                    `json:"network_tags,omitempty" jsonschema:"description=A list of network tags to be attached to the instance"`
	ServiceAccounts []*computepb.ServiceAccount `json:"service_accounts,omitempty" jsonschema:"description=A list of service accounts to be attached to the instance"`
	SourceSnapshot  string                      `json:"source_snapshot,omitempty" jsonschema:"description=The source snapshot to create this disk."`
	SSHKeys         []string                    `json:"ssh_keys,omitempty" jsonschema:"description=A list of SSH keys to be added to the instance. The format is USERNAME:SSH_KEY"`
	EnableBootDebug *bool                       `json:"enable_boot_debug,omitempty" jsonschema:"description=Enable boot debug on the VM."`
	DisableUpdates  *bool                       `json:"disable_updates,omitempty" jsonschema:"description=Disable OS updates on boot."`
	// Shielded VM options
	EnableSecureBoot          *bool `json:"enable_secure_boot,omitempty" jsonschema:"description=Enable Secure Boot on the VM. Requires a Shielded VM compatible image."`
	EnableVTPM                *bool `json:"enable_vtpm,omitempty" jsonschema:"description=Enable virtual Trusted Platform Module (vTPM) on the VM."`
	EnableIntegrityMonitoring *bool `json:"enable_integrity_monitoring,omitempty" jsonschema:"description=Enable integrity monitoring on the VM."`
	// The Cloudconfig struct from common package
	cloudconfig.CloudConfigSpec
}

func GetRunnerSpecFromBootstrapParams(cfg *config.Config, data params.BootstrapInstance, controllerID string) (*RunnerSpec, error) {
	tools, err := DefaultToolFetch(data.OSType, data.OSArch, data.Tools)
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
	DisplayDevice   bool
	DiskSize        int64
	DiskType        string
	CustomLabels    map[string]string
	NetworkTags     []string
	ServiceAccounts []*computepb.ServiceAccount
	SourceSnapshot  string
	SSHKeys         string
	EnableBootDebug bool
	DisableUpdates  bool
	// Shielded VM options
	EnableSecureBoot          bool
	EnableVTPM                bool
	EnableIntegrityMonitoring bool
}

func (r *RunnerSpec) MergeExtraSpecs(extraSpecs *extraSpecs) {
	if extraSpecs.NetworkID != "" {
		r.NetworkID = extraSpecs.NetworkID
	}
	if extraSpecs.SubnetworkID != "" {
		r.SubnetworkID = extraSpecs.SubnetworkID
	}
	if extraSpecs.DisplayDevice {
		r.DisplayDevice = extraSpecs.DisplayDevice
	}
	if extraSpecs.DiskSize > 0 {
		r.DiskSize = extraSpecs.DiskSize
	}
	if extraSpecs.DiskType != "" {
		r.DiskType = extraSpecs.DiskType
	}
	if extraSpecs.NicType != "" {
		r.NicType = extraSpecs.NicType
	}
	if len(extraSpecs.CustomLabels) > 0 {
		maps.Copy(r.CustomLabels, extraSpecs.CustomLabels)
	}
	if len(extraSpecs.NetworkTags) > 0 {
		r.NetworkTags = extraSpecs.NetworkTags
	}
	if len(extraSpecs.ServiceAccounts) > 0 {
		r.ServiceAccounts = extraSpecs.ServiceAccounts
	}
	if extraSpecs.SourceSnapshot != "" {
		r.SourceSnapshot = extraSpecs.SourceSnapshot
	}
	if len(extraSpecs.SSHKeys) > 0 {
		for key := range extraSpecs.SSHKeys {
			r.SSHKeys = r.SSHKeys + "\n" + extraSpecs.SSHKeys[key]
		}
	}
	if extraSpecs.EnableBootDebug != nil {
		r.EnableBootDebug = *extraSpecs.EnableBootDebug
	}
	if extraSpecs.DisableUpdates != nil {
		r.DisableUpdates = *extraSpecs.DisableUpdates
	}
	if extraSpecs.EnableSecureBoot != nil {
		r.EnableSecureBoot = *extraSpecs.EnableSecureBoot
	}
	if extraSpecs.EnableVTPM != nil {
		r.EnableVTPM = *extraSpecs.EnableVTPM
	}
	if extraSpecs.EnableIntegrityMonitoring != nil {
		r.EnableIntegrityMonitoring = *extraSpecs.EnableIntegrityMonitoring
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
	bootstrapParams := r.BootstrapParams
	bootstrapParams.UserDataOptions.EnableBootDebug = r.EnableBootDebug
	bootstrapParams.UserDataOptions.DisableUpdatesOnBoot = r.DisableUpdates

	switch r.BootstrapParams.OSType {
	case params.Linux:
		// Get the cloud-init config
		udata, err := DefaultCloudConfigFunc(bootstrapParams, r.Tools, bootstrapParams.Name)
		if err != nil {
			return "", fmt.Errorf("failed to generate userdata: %w", err)
		}
		return udata, nil

	case params.Windows:
		udata, err := DefaultRunnerInstallScriptFunc(bootstrapParams, r.Tools, bootstrapParams.Name)
		if err != nil {
			return "", fmt.Errorf("failed to generate userdata: %w", err)
		}
		return string(udata), nil
	}
	return "", fmt.Errorf("unsupported OS type for cloud config: %s", r.BootstrapParams.OSType)
}
