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

package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-gcp/internal/spec"
	"github.com/cloudbase/garm-provider-gcp/internal/util"
	"github.com/google/uuid"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

const (
	regionalTargetShape string = "ANY_SINGLE_ZONE"

	ambiguousCreateLookupTimeout  = 30 * time.Second
	ambiguousCreateLookupInterval = time.Second
)

func (g *GcpCli) createRegionalInstance(ctx context.Context, runnerSpec *spec.RunnerSpec, inst *computepb.Instance) (*computepb.Instance, error) {
	if g.regionClient == nil {
		return nil, fmt.Errorf("regional placement client is not configured")
	}
	markRegionalInstance(inst)
	existing, err := g.findInstanceInZones(ctx, inst.GetName(), runnerSpec.RegionalPlacement.Zones)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing regional instance %s: %w", inst.GetName(), err)
	}
	if existing != nil {
		if err := validateRegionalInstanceIdentity(existing, inst); err != nil {
			return nil, fmt.Errorf("existing regional instance %s does not match this runner: %w", inst.GetName(), err)
		}
		return existing, nil
	}

	req := buildRegionalInsertRequest(g.cfg.ProjectId, runnerSpec, inst)
	op, err := g.regionClient.BulkInsert(ctx, req)
	if err == nil {
		err = WaitOp(op, ctx)
	}
	if err != nil {
		if !isAmbiguousCreateError(err) {
			return nil, fmt.Errorf("failed to create regional instance: %w", err)
		}
		// The request may have succeeded on the GCP side. Look for the instance
		// before reporting the error, so we don't leak a running instance.
		lookupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ambiguousCreateLookupTimeout)
		defer cancel()
		created, lookupErr := g.waitForInstanceInZones(lookupCtx, inst.GetName(), runnerSpec.RegionalPlacement.Zones)
		if lookupErr != nil {
			return nil, fmt.Errorf("failed to reconcile regional create error %w: %w", err, lookupErr)
		}
		if created == nil {
			return nil, fmt.Errorf("regional create result is ambiguous: %w", err)
		}
		if identityErr := validateRegionalInstanceIdentity(created, inst); identityErr != nil {
			return nil, fmt.Errorf("regional create returned a mismatched instance: %w", identityErr)
		}
		return created, nil
	}

	created, err := g.findInstanceInZones(ctx, inst.GetName(), runnerSpec.RegionalPlacement.Zones)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve created regional instance %s: %w", inst.GetName(), err)
	}
	if created == nil {
		return nil, fmt.Errorf("regional bulk insert completed without instance %s", inst.GetName())
	}
	if err := validateRegionalInstanceIdentity(created, inst); err != nil {
		return nil, fmt.Errorf("regional create returned a mismatched instance: %w", err)
	}
	return created, nil
}

func buildRegionalInsertRequest(project string, runnerSpec *spec.RunnerSpec, inst *computepb.Instance) *computepb.BulkInsertRegionInstanceRequest {
	disks := make([]*computepb.AttachedDisk, 0, len(inst.Disks))
	for _, disk := range inst.Disks {
		cloned := proto.Clone(disk).(*computepb.AttachedDisk)
		if cloned.InitializeParams != nil {
			cloned.InitializeParams.SourceSnapshot = nil
		}
		disks = append(disks, cloned)
	}
	zones := make([]*computepb.LocationPolicyZoneConfiguration, 0, len(runnerSpec.RegionalPlacement.Zones))
	for _, zone := range runnerSpec.RegionalPlacement.Zones {
		zones = append(zones, &computepb.LocationPolicyZoneConfiguration{
			Zone: proto.String("zones/" + zone),
		})
	}
	return &computepb.BulkInsertRegionInstanceRequest{
		Project:   project,
		Region:    runnerSpec.RegionalPlacement.Region(),
		RequestId: proto.String(uuid.NewString()),
		BulkInsertInstanceResourceResource: &computepb.BulkInsertInstanceResource{
			Count:    proto.Int64(1),
			MinCount: proto.Int64(1),
			LocationPolicy: &computepb.LocationPolicy{
				TargetShape: proto.String(regionalTargetShape),
				Zones:       zones,
			},
			InstanceProperties: &computepb.InstanceProperties{
				MachineType:            proto.String(runnerSpec.BootstrapParams.Flavor),
				Disks:                  disks,
				Labels:                 inst.Labels,
				Metadata:               inst.Metadata,
				NetworkInterfaces:      inst.NetworkInterfaces,
				ServiceAccounts:        inst.ServiceAccounts,
				ShieldedInstanceConfig: inst.ShieldedInstanceConfig,
				Tags:                   inst.Tags,
			},
			PerInstanceProperties: map[string]*computepb.BulkInsertInstanceResourcePerInstanceProperties{
				inst.GetName(): {},
			},
		},
	}
}

func markRegionalInstance(inst *computepb.Instance) {
	if inst.Labels == nil {
		inst.Labels = make(map[string]string)
	}
	inst.Labels[util.RegionalPlacementLabel] = "true"
}

func validateRegionalInstanceIdentity(existing, expected *computepb.Instance) error {
	if existing.GetName() != expected.GetName() {
		return fmt.Errorf("name is %s, expected %s", existing.GetName(), expected.GetName())
	}
	if existing.Labels[util.RegionalPlacementLabel] != "true" {
		return fmt.Errorf("regional placement marker is missing")
	}
	for key, value := range expected.Labels {
		if existing.Labels[key] != value {
			return fmt.Errorf("label %s is %s, expected %s", key, existing.Labels[key], value)
		}
	}
	return nil
}

func splitRegionalProviderID(providerID string) (string, string, bool) {
	zone, name, ok := strings.Cut(providerID, "/")
	if !ok || zone == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return strings.ToLower(zone), util.GetInstanceName(name), true
}

func (g *GcpCli) getInstanceInZone(ctx context.Context, zone, name string) (*computepb.Instance, error) {
	req := &computepb.GetInstanceRequest{
		Project:  g.cfg.ProjectId,
		Zone:     zone,
		Instance: util.GetInstanceName(name),
	}

	instance, err := g.client.Get(ctx, req)
	if err != nil {
		return nil, err
	}
	if instance != nil && instance.Zone == nil {
		instance.Zone = proto.String("zones/" + zone)
	}
	return instance, nil
}

func (g *GcpCli) findInstanceInZones(ctx context.Context, name string, zones []string) (*computepb.Instance, error) {
	var found *computepb.Instance
	for _, zone := range zones {
		instance, err := g.getInstanceInZone(ctx, zone, name)
		if err != nil {
			if isRegionalNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("failed to search zone %s: %w", zone, err)
		}
		if instance == nil {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("instance name %s exists in multiple zones", name)
		}
		found = instance
	}
	return found, nil
}

func (g *GcpCli) waitForInstanceInZones(ctx context.Context, name string, zones []string) (*computepb.Instance, error) {
	for {
		instance, err := g.findInstanceInZones(ctx, name, zones)
		if err != nil || instance != nil {
			return instance, err
		}
		timer := time.NewTimer(ambiguousCreateLookupInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, nil
		case <-timer.C:
		}
	}
}

func (g *GcpCli) listRegionalInstances(ctx context.Context, poolID string) ([]*computepb.Instance, error) {
	filter := fmt.Sprintf("labels.garmpoolid=%s AND labels.%s=true", poolID, util.RegionalPlacementLabel)
	req := &computepb.AggregatedListInstancesRequest{
		Project:              g.cfg.ProjectId,
		Filter:               &filter,
		ReturnPartialSuccess: proto.Bool(true),
	}

	it := g.client.AggregatedList(ctx, req)
	var instances []*computepb.Instance
	for {
		pair, err := NextAggregatedIt(it)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		if pair.Value == nil {
			continue
		}
		for _, instance := range pair.Value.Instances {
			if instance.Labels[util.RegionalPlacementLabel] != "true" {
				continue
			}
			if instance.Zone == nil && strings.HasPrefix(pair.Key, "zones/") {
				instance.Zone = proto.String(pair.Key)
			}
			instances = append(instances, instance)
		}
	}
	return instances, nil
}

func isRegionalNotFound(err error) bool {
	asApiErr, ok := err.(*apierror.APIError)
	return ok && asApiErr.HTTPCode() == 404
}

func isAmbiguousCreateError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, reason := range []string{"unexpected eof", "connection reset", "transport is closing", "client connection lost"} {
		if strings.Contains(message, reason) {
			return true
		}
	}
	return false
}
