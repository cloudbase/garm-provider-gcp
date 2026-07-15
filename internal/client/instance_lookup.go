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

package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-gcp/internal/util"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

func splitProviderID(providerID string) (zone, name string, zoned bool) {
	zone, name, zoned = strings.Cut(providerID, "/")
	if !zoned || zone == "" || name == "" || strings.Contains(name, "/") {
		return "", util.GetInstanceName(providerID), false
	}
	return zone, util.GetInstanceName(name), true
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
			if isNotFoundError(err) {
				continue
			}
			return nil, fmt.Errorf("failed to search zone %q: %w", zone, err)
		}
		if instance == nil {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("instance name %q exists in multiple zones", name)
		}
		found = instance
	}
	return found, nil
}

func (g *GcpCli) findInstanceAggregated(ctx context.Context, name string) (*computepb.Instance, error) {
	filter := fmt.Sprintf("name = %q", util.GetInstanceName(name))
	it := g.client.AggregatedList(ctx, &computepb.AggregatedListInstancesRequest{
		Project: g.cfg.ProjectId,
		Filter:  &filter,
	})
	var found *computepb.Instance
	for {
		pair, err := NextAggregatedIt(it)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list instances: %w", err)
		}
		if pair.Value == nil {
			continue
		}
		for _, instance := range pair.Value.Instances {
			if instance.GetName() != util.GetInstanceName(name) {
				continue
			}
			if found != nil {
				return nil, fmt.Errorf("instance name %q exists in multiple zones", name)
			}
			if instance.Zone == nil && strings.HasPrefix(pair.Key, "zones/") {
				instance.Zone = proto.String(pair.Key)
			}
			found = instance
		}
	}
	return found, nil
}

func (g *GcpCli) listInstancesAggregated(ctx context.Context, poolID string) ([]*computepb.Instance, error) {
	filter := fmt.Sprintf("labels.garmpoolid=%s", poolID)
	it := g.client.AggregatedList(ctx, &computepb.AggregatedListInstancesRequest{
		Project: g.cfg.ProjectId,
		Filter:  &filter,
	})
	var instances []*computepb.Instance
	for {
		pair, err := NextAggregatedIt(it)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list instances: %w", err)
		}
		if pair.Value == nil {
			continue
		}
		for _, instance := range pair.Value.Instances {
			if instance.Zone == nil && strings.HasPrefix(pair.Key, "zones/") {
				instance.Zone = proto.String(pair.Key)
			}
			instances = append(instances, instance)
		}
	}
	return instances, nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPCode() == 404 {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "notfound") || strings.Contains(message, "code = 404")
}
