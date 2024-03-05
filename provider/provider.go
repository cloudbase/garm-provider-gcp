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

package provider

import (
	"context"
	"fmt"

	"github.com/cloudbase/garm-provider-common/execution"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-gcp/config"
	"github.com/cloudbase/garm-provider-gcp/internal/client"
	"github.com/cloudbase/garm-provider-gcp/internal/spec"
	"github.com/cloudbase/garm-provider-gcp/internal/util"
)

var _ execution.ExternalProvider = &GcpProvider{}

func NewGcpProvider(ctx context.Context, cfgFile string, controllerID string) (*GcpProvider, error) {
	conf, err := config.NewConfig(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}

	gcpCli, err := client.NewGcpCli(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("error creating GCP client: %w", err)
	}

	return &GcpProvider{
		gcpCli:       gcpCli,
		controllerID: controllerID,
	}, nil
}

type GcpProvider struct {
	gcpCli       *client.GcpCli
	controllerID string
}

func (g *GcpProvider) CreateInstance(ctx context.Context, bootstrapParams params.BootstrapInstance) (params.ProviderInstance, error) {
	spec, err := spec.GetRunnerSpecFromBootstrapParams(g.gcpCli.Config(), bootstrapParams, g.controllerID)
	if err != nil {
		return params.ProviderInstance{}, fmt.Errorf("failed to get runner spec: %w", err)
	}
	inst, err := g.gcpCli.CreateInstance(ctx, spec)
	if err != nil {
		return g.GetInstance(ctx, bootstrapParams.Name)
	}
	instance := params.ProviderInstance{
		ProviderID: *inst.Name,
		Name:       spec.BootstrapParams.Name,
		OSType:     spec.BootstrapParams.OSType,
		OSArch:     spec.BootstrapParams.OSArch,
		Status:     "running",
	}
	return instance, nil
}

func (g *GcpProvider) GetInstance(ctx context.Context, instance string) (params.ProviderInstance, error) {
	inst, err := g.gcpCli.GetInstance(ctx, instance)
	if err != nil {
		return params.ProviderInstance{}, fmt.Errorf("error getting instance: %w", err)
	}
	instanceParams, err := util.GcpInstanceToParamsInstance(inst)
	if err != nil {
		return params.ProviderInstance{}, fmt.Errorf("error converting instance: %w", err)
	}
	return instanceParams, nil
}

func (g *GcpProvider) DeleteInstance(ctx context.Context, instance string) error {
	err := g.gcpCli.DeleteInstance(ctx, instance)
	if err != nil {
		return fmt.Errorf("error deleting instance: %w", err)
	}
	return nil
}

func (g *GcpProvider) ListInstances(ctx context.Context, poolID string) ([]params.ProviderInstance, error) {
	gcpInstances, err := g.gcpCli.ListDescribedInstances(ctx, poolID)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	var providerInstances []params.ProviderInstance
	for _, val := range gcpInstances {
		inst, err := util.GcpInstanceToParamsInstance(val)
		if err != nil {
			return []params.ProviderInstance{}, fmt.Errorf("failed to convert instance: %w", err)
		}
		providerInstances = append(providerInstances, inst)
	}
	return providerInstances, nil
}

func (g *GcpProvider) RemoveAllInstances(ctx context.Context) error {
	return nil
}

func (g *GcpProvider) Stop(ctx context.Context, instance string, force bool) error {
	return g.gcpCli.StopInstance(ctx, instance)
}

func (g *GcpProvider) Start(ctx context.Context, instance string) error {
	return g.gcpCli.StartInstance(ctx, instance)
}
