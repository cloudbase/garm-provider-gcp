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
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-gcp/internal/spec"
	"github.com/cloudbase/garm-provider-gcp/internal/util"
	"google.golang.org/protobuf/proto"
)

const quotaAdvanceLogMarker = "gcp_capacity_policy_quota_advance"

const ambiguousCreateLookupTimeout = 30 * time.Second

type rankedCandidate struct {
	candidate spec.CapacityCandidate
	rank      int
}

type placementAttempt struct {
	model      string
	zones      []string
	candidates []rankedCandidate
}

type candidateFailure struct {
	model     string
	candidate spec.CapacityCandidate
	zones     []string
	err       error
}

func buildPlacementAttempts(policy *spec.CapacityPolicy) []placementAttempt {
	var attempts []placementAttempt
	for _, model := range policy.ProvisioningModels {
		for index := 0; index < len(policy.Candidates); {
			zones := effectiveCandidateZones(policy, policy.Candidates[index])
			attempt := placementAttempt{model: model, zones: zones}
			for index < len(policy.Candidates) && slices.Equal(zones, effectiveCandidateZones(policy, policy.Candidates[index])) {
				attempt.candidates = append(attempt.candidates, rankedCandidate{
					candidate: policy.Candidates[index],
					rank:      index,
				})
				index++
			}
			attempts = append(attempts, attempt)
		}
	}
	return attempts
}

func effectiveCandidateZones(policy *spec.CapacityPolicy, candidate spec.CapacityCandidate) []string {
	if len(candidate.Zones) == 0 {
		return slices.Clone(policy.Zones)
	}

	// Candidate zones are a compatibility set, not a second placement order.
	// Keep the policy's zone order so equivalent sets share one flexibility
	// request and candidate rank remains the only machine preference.
	zones := make([]string, 0, len(candidate.Zones))
	for _, zone := range policy.Zones {
		if slices.Contains(candidate.Zones, zone) {
			zones = append(zones, zone)
		}
	}
	return zones
}

func (g *GcpCli) createCapacityInstance(ctx context.Context, runnerSpec *spec.RunnerSpec, inst *computepb.Instance) (*computepb.Instance, error) {
	markCapacityPolicyInstance(inst)
	existing, err := g.findInstanceInZones(ctx, inst.GetName(), runnerSpec.CapacityPolicy.Zones)
	if err != nil {
		return nil, fmt.Errorf("failed to check for an existing regional instance %q: %w", inst.GetName(), err)
	}
	if existing != nil {
		if err := validateCapacityInstanceIdentity(existing, inst); err != nil {
			return nil, fmt.Errorf("existing regional instance %q does not match this runner: %w", inst.GetName(), err)
		}
		return existing, nil
	}
	attempts := buildPlacementAttempts(runnerSpec.CapacityPolicy)
	var failures []candidateFailure
	modelHadQuota := false
	currentModel := ""

	for _, attempt := range attempts {
		if currentModel != attempt.model {
			if currentModel != "" && modelHadQuota {
				return nil, aggregateCandidateFailures(failures)
			}
			currentModel = attempt.model
			modelHadQuota = false
		}

		remaining := slices.Clone(attempt.candidates)
		for len(remaining) > 0 {
			req, err := buildBulkInsertRequest(g.cfg.ProjectId, runnerSpec, inst, attempt.model, attempt.zones, remaining)
			if err != nil {
				return nil, fmt.Errorf("failed to build regional bulk insert request: %w", err)
			}
			err = g.bulkInsertInstance(ctx, req)
			if err == nil {
				created, lookupErr := g.findInstanceInZones(ctx, inst.GetName(), attempt.zones)
				if lookupErr != nil {
					return nil, fmt.Errorf("failed to resolve created instance %q: %w", inst.GetName(), lookupErr)
				}
				if created == nil {
					return nil, fmt.Errorf("regional bulk insert completed without instance %q", inst.GetName())
				}
				return created, nil
			}

			lookupCtx := ctx
			cancelLookup := func() {}
			if isAmbiguousCreateError(err) {
				// A timeout or canceled create context cannot be reused to determine
				// whether Compute Engine accepted the request. Preserve request values
				// while detaching cancellation, and bound the reconciliation read so an
				// uncertain create never advances into a duplicate placement.
				lookupCtx, cancelLookup = context.WithTimeout(context.WithoutCancel(ctx), ambiguousCreateLookupTimeout)
			}
			created, lookupErr := g.findInstanceInZones(lookupCtx, inst.GetName(), attempt.zones)
			cancelLookup()
			if lookupErr != nil {
				return nil, fmt.Errorf("failed to reconcile create error %w: lookup failed: %w", err, lookupErr)
			}
			if created != nil {
				return created, nil
			}

			switch classifyPlacementError(err) {
			case placementErrorQuota:
				candidate := remaining[0]
				failures = append(failures, candidateFailure{model: attempt.model, candidate: candidate.candidate, zones: attempt.zones, err: err})
				log.Printf("%s model=%s machine_type=%s rank=%d error=%v", quotaAdvanceLogMarker, attempt.model, candidate.candidate.MachineType, candidate.rank, err)
				modelHadQuota = true
				remaining = remaining[1:]
			case placementErrorCapacity:
				for _, candidate := range remaining {
					failures = append(failures, candidateFailure{model: attempt.model, candidate: candidate.candidate, zones: attempt.zones, err: err})
				}
				remaining = nil
			default:
				return nil, err
			}
		}
	}

	return nil, aggregateCandidateFailures(failures)
}

func validateCapacityInstanceIdentity(existing, expected *computepb.Instance) error {
	if existing.GetName() != expected.GetName() {
		return fmt.Errorf("name is %q, expected %q", existing.GetName(), expected.GetName())
	}
	if !hasMetadataValue(existing.Metadata, util.CapacityPolicyMetadataKey, "true") {
		return fmt.Errorf("capacity policy marker is missing")
	}
	for key, value := range expected.Labels {
		if existing.Labels[key] != value {
			return fmt.Errorf("label %q is %q, expected %q", key, existing.Labels[key], value)
		}
	}
	return nil
}

func hasMetadataValue(metadata *computepb.Metadata, key, value string) bool {
	if metadata == nil {
		return false
	}
	for _, item := range metadata.Items {
		if item.GetKey() == key && item.GetValue() == value {
			return true
		}
	}
	return false
}

func buildBulkInsertRequest(project string, runnerSpec *spec.RunnerSpec, inst *computepb.Instance, model string, zones []string, candidates []rankedCandidate) (*computepb.BulkInsertRegionInstanceRequest, error) {
	markCapacityPolicyInstance(inst)
	requestID, err := newRequestID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate request ID: %w", err)
	}

	selections := make(map[string]*computepb.InstanceFlexibilityPolicyInstanceSelection, len(candidates))
	for _, ranked := range candidates {
		candidate := ranked.candidate
		image := candidate.Image
		if image == "" {
			image = runnerSpec.BootstrapParams.Image
		}
		diskType := candidate.DiskType
		if diskType == "" {
			diskType = runnerSpec.DiskType
		}
		diskSize := candidate.DiskSize
		if diskSize == 0 {
			diskSize = runnerSpec.DiskSize
		}
		snapshot := runnerSpec.SourceSnapshot
		if candidate.Image != "" {
			snapshot = ""
		}
		disks := generateBootDisk(diskSize, image, snapshot, diskType, runnerSpec.CustomLabels, runnerSpec.BootDiskKmsKeyName)
		disks[0].InitializeParams.Architecture = proto.String(gceDiskArchitecture(candidate.Architecture))
		selections[fmt.Sprintf("selection-%03d", ranked.rank)] = &computepb.InstanceFlexibilityPolicyInstanceSelection{
			Rank:         proto.Int64(int64(ranked.rank)),
			MachineTypes: []string{candidate.MachineType},
			Disks:        disks,
		}
	}

	locationZones := make([]*computepb.LocationPolicyZoneConfiguration, 0, len(zones))
	for _, zone := range zones {
		locationZones = append(locationZones, &computepb.LocationPolicyZoneConfiguration{Zone: proto.String("zones/" + zone)})
	}

	properties := &computepb.InstanceProperties{
		Labels:                 inst.Labels,
		Metadata:               inst.Metadata,
		NetworkInterfaces:      inst.NetworkInterfaces,
		Scheduling:             schedulingForModel(model),
		ServiceAccounts:        inst.ServiceAccounts,
		ShieldedInstanceConfig: inst.ShieldedInstanceConfig,
		Tags:                   inst.Tags,
	}
	one := int64(1)
	return &computepb.BulkInsertRegionInstanceRequest{
		Project:   project,
		Region:    runnerSpec.CapacityPolicy.Region(),
		RequestId: proto.String(requestID),
		BulkInsertInstanceResourceResource: &computepb.BulkInsertInstanceResource{
			Count:    &one,
			MinCount: &one,
			LocationPolicy: &computepb.LocationPolicy{
				TargetShape: proto.String("ANY_SINGLE_ZONE"),
				Zones:       locationZones,
			},
			InstanceProperties: properties,
			InstanceFlexibilityPolicy: &computepb.InstanceFlexibilityPolicy{
				InstanceSelections: selections,
			},
			PerInstanceProperties: map[string]*computepb.BulkInsertInstanceResourcePerInstanceProperties{
				inst.GetName(): {},
			},
		},
	}, nil
}

func markCapacityPolicyInstance(inst *computepb.Instance) {
	if inst.Metadata == nil {
		inst.Metadata = &computepb.Metadata{}
	}
	for _, item := range inst.Metadata.Items {
		if item.GetKey() == util.CapacityPolicyMetadataKey {
			item.Value = proto.String("true")
			return
		}
	}
	inst.Metadata.Items = append(inst.Metadata.Items, &computepb.Items{
		Key: proto.String(util.CapacityPolicyMetadataKey), Value: proto.String("true"),
	})
}

func schedulingForModel(model string) *computepb.Scheduling {
	if model != "SPOT" {
		return nil
	}
	inst := &computepb.Instance{}
	setSpotScheduling(inst)
	inst.Scheduling.Preemptible = proto.Bool(true)
	return inst.Scheduling
}

func gceDiskArchitecture(architecture params.OSArch) string {
	if architecture == params.Arm64 {
		return "ARM64"
	}
	return "X86_64"
}

func (g *GcpCli) bulkInsertInstance(ctx context.Context, req *computepb.BulkInsertRegionInstanceRequest) error {
	op, err := g.regionClient.BulkInsert(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create regional instance: %w", err)
	}
	if err := WaitOp(op, ctx); err != nil {
		return fmt.Errorf("failed to wait for regional create operation: %w", err)
	}
	return nil
}

type placementErrorClass int

const (
	placementErrorTerminal placementErrorClass = iota
	placementErrorCapacity
	placementErrorQuota
)

func classifyPlacementError(err error) placementErrorClass {
	if isAmbiguousCreateError(err) {
		return placementErrorTerminal
	}
	if isQuotaError(err) {
		return placementErrorQuota
	}
	if isCapacityError(err) {
		return placementErrorCapacity
	}
	return placementErrorTerminal
}

func isAmbiguousCreateError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := strings.ToLower(err.Error())
	ambiguousReasons := []string{
		"context canceled",
		"context deadline exceeded",
		"unexpected eof",
		"connection reset",
		"transport is closing",
		"client connection lost",
	}
	for _, reason := range ambiguousReasons {
		if strings.Contains(message, reason) {
			return true
		}
	}
	return false
}

func isQuotaError(err error) bool {
	if hasPlacementErrorReason(err, []string{"quotaexceeded"}) {
		return true
	}
	if len(structuredPlacementErrorReasons(err)) > 0 {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "quota") && strings.Contains(message, "exceeded")
}

func aggregateCandidateFailures(failures []candidateFailure) error {
	if len(failures) == 0 {
		return fmt.Errorf("capacity policy exhausted without an attempt")
	}
	reasons := make([]error, 0, len(failures))
	for _, failure := range failures {
		reasons = append(reasons, fmt.Errorf("model=%s machine_type=%s zones=%s: %w", failure.model, failure.candidate.MachineType, strings.Join(failure.zones, ","), failure.err))
	}
	return fmt.Errorf("capacity policy exhausted: %w", errors.Join(reasons...))
}

func newRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
