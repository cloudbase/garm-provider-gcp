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
	"fmt"
	"os"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm-provider-gcp/config"
	"github.com/cloudbase/garm-provider-gcp/internal/spec"
	"github.com/cloudbase/garm-provider-gcp/internal/util"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"golang.org/x/oauth2/google"
	gcompute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

const (
	linuxUserData        string = "user-data"
	windowsStartupScript string = "sysprep-specialize-script-ps1"
	accessConfigType     string = "ONE_TO_ONE_NAT"
)

var (
	WaitOp = (*compute.Operation).Wait
	NextIt = (*compute.InstanceIterator).Next
)

func getHTTPClientOptionFromCredentialsFile(ctx context.Context, credentialsFile string) (option.ClientOption, error) {
	jsonKey, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON key file: %w", err)
	}
	config, err := google.JWTConfigFromJSON(jsonKey, gcompute.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT config: %w", err)
	}
	// Create an HTTP client using the JWT Config
	client := config.Client(ctx)

	return option.WithHTTPClient(client), nil
}

func NewGcpCli(ctx context.Context, cfg *config.Config) (*GcpCli, error) {
	var authOptions []option.ClientOption

	if cfg.CredentialsFile != "" {
		clientOption, err := getHTTPClientOptionFromCredentialsFile(ctx, cfg.CredentialsFile)
		if err != nil {
			// Explicit credentials were set, but failed to create the client.
			return nil, fmt.Errorf("failed to get http client option: %w", err)
		}
		authOptions = append(authOptions, clientOption)
	}
	creds, err := google.FindDefaultCredentials(ctx, gcompute.CloudPlatformScope)
	if err != nil && len(authOptions) == 0 {
		return nil, fmt.Errorf("failed to find default credentials and no credentials file supplied: %w", err)
	}
	authOptions = append(authOptions, option.WithCredentials(creds))

	// Now use this client to create a Compute Engine client
	computeClient, err := compute.NewInstancesRESTClient(ctx, authOptions...)
	if err != nil {
		return nil, fmt.Errorf("error creating compute service: %w", err)
	}
	gcpCli := &GcpCli{
		cfg:    cfg,
		client: computeClient,
	}

	return gcpCli, nil
}

type ClientInterface interface {
	Insert(ctx context.Context, req *computepb.InsertInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error)
	Start(ctx context.Context, req *computepb.StartInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error)
	Stop(ctx context.Context, req *computepb.StopInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error)
	Delete(ctx context.Context, req *computepb.DeleteInstanceRequest, opts ...gax.CallOption) (*compute.Operation, error)
	List(ctx context.Context, req *computepb.ListInstancesRequest, opts ...gax.CallOption) *compute.InstanceIterator
	Get(ctx context.Context, req *computepb.GetInstanceRequest, opts ...gax.CallOption) (*computepb.Instance, error)
}

type GcpCli struct {
	cfg    *config.Config
	client ClientInterface
}

func (g GcpCli) Config() *config.Config {
	return g.cfg
}

func (g GcpCli) Client() ClientInterface {
	return g.client
}

func (g *GcpCli) SetClient(client ClientInterface) {
	g.client = client
}

func (g *GcpCli) SetConfig(cfg *config.Config) {
	g.cfg = cfg
}

func (g *GcpCli) CreateInstance(ctx context.Context, spec *spec.RunnerSpec) (*computepb.Instance, error) {
	if spec == nil {
		return nil, fmt.Errorf("invalid nil runner spec")
	}

	udata, err := spec.ComposeUserData()
	if err != nil {
		return nil, fmt.Errorf("failed to compose user data: %w", err)
	}

	name := util.GetInstanceName(spec.BootstrapParams.Name)

	inst := &computepb.Instance{
		Name:        proto.String(name),
		MachineType: proto.String(util.GetMachineType(g.cfg.Zone, spec.BootstrapParams.Flavor)),
		Disks:       generateBootDisk(spec.DiskSize, spec.BootstrapParams.Image, spec.SourceSnapshot, spec.DiskType, spec.CustomLabels),
		DisplayDevice: &computepb.DisplayDevice{
			EnableDisplay: proto.Bool(spec.DisplayDevice),
		},
		NetworkInterfaces: []*computepb.NetworkInterface{
			{
				Network: proto.String(g.cfg.NetworkID),
				NicType: proto.String(spec.NicType),
				AccessConfigs: []*computepb.AccessConfig{
					{
						// The type of configuration. In accessConfigs (IPv4), the default and only option is ONE_TO_ONE_NAT.
						Type: proto.String(accessConfigType),
					},
				},
				Subnetwork: &spec.SubnetworkID,
			},
		},
		Metadata: &computepb.Metadata{
			Items: []*computepb.Items{
				{
					Key:   proto.String(selectStartupScript(spec.BootstrapParams.OSType)),
					Value: proto.String(udata),
				},
				{
					Key:   proto.String("runner_name"),
					Value: proto.String(spec.BootstrapParams.Name),
				},
				{
					Key:   proto.String("ssh-keys"),
					Value: proto.String(spec.SSHKeys),
				},
			},
		},
		Labels: spec.CustomLabels,
		Tags: &computepb.Tags{
			Items: spec.NetworkTags,
		},
		ServiceAccounts: spec.ServiceAccounts,
		ShieldedInstanceConfig: &computepb.ShieldedInstanceConfig{
			EnableSecureBoot:          proto.Bool(spec.EnableSecureBoot),
			EnableVtpm:                proto.Bool(spec.EnableVTPM),
			EnableIntegrityMonitoring: proto.Bool(spec.EnableIntegrityMonitoring),
		},
	}

	if !g.cfg.ExternalIPAccess {
		inst.NetworkInterfaces[0].AccessConfigs = nil
	}

	if spec.BootstrapParams.OSType == params.Windows && len(spec.SSHKeys) > 0 {
		inst.Metadata.Items = append(inst.Metadata.Items, &computepb.Items{
			Key:   proto.String("enable-windows-ssh"),
			Value: proto.String("TRUE"),
		})
		inst.Metadata.Items = append(inst.Metadata.Items, &computepb.Items{
			Key:   proto.String("sysprep-specialize-script-cmd"),
			Value: proto.String("googet -noconfirm=true install google-compute-engine-ssh"),
		})
	}

	insertReq := &computepb.InsertInstanceRequest{
		Project:          g.cfg.ProjectId,
		Zone:             g.cfg.Zone,
		InstanceResource: inst,
	}

	op, err := g.client.Insert(ctx, insertReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance %s: %w", insertReq, err)
	}

	if err = WaitOp(op, ctx); err != nil {
		return nil, fmt.Errorf("failed to wait for operation: %w", err)
	}

	return inst, nil
}

func (g *GcpCli) GetInstance(ctx context.Context, instanceName string) (*computepb.Instance, error) {
	req := &computepb.GetInstanceRequest{
		Project:  g.cfg.ProjectId,
		Zone:     g.cfg.Zone,
		Instance: util.GetInstanceName(instanceName),
	}

	instance, err := g.client.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %v", err)
	}

	return instance, nil
}

func (g *GcpCli) ListDescribedInstances(ctx context.Context, poolID string) ([]*computepb.Instance, error) {
	label := fmt.Sprintf("labels.garmpoolid=%s", poolID)
	req := &computepb.ListInstancesRequest{
		Project: g.cfg.ProjectId,
		Zone:    g.cfg.Zone,
		Filter:  &label,
	}

	it := g.client.List(ctx, req)
	var instances []*computepb.Instance
	for {
		instance, _ := NextIt(it)
		if instance == nil {
			break
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

func (g *GcpCli) DeleteInstance(ctx context.Context, instance string) error {
	req := &computepb.DeleteInstanceRequest{
		Instance: util.GetInstanceName(instance),
		Project:  g.cfg.ProjectId,
		Zone:     g.cfg.Zone,
	}

	op, err := g.client.Delete(ctx, req)

	if err != nil {
		asApiErr, ok := err.(*apierror.APIError)
		if ok && asApiErr.HTTPCode() == 404 {
			// We got a 404 error. The instance is gone.
			return nil
		}
		return fmt.Errorf("unable to delete instance: %w", err)
	}

	if err = WaitOp(op, ctx); err != nil {
		return fmt.Errorf("unable to wait for the delete operation: %w", err)
	}

	return nil
}

func (g *GcpCli) StopInstance(ctx context.Context, instance string) error {
	req := &computepb.StopInstanceRequest{
		Instance: util.GetInstanceName(instance),
		Project:  g.cfg.ProjectId,
		Zone:     g.cfg.Zone,
	}

	op, err := g.client.Stop(ctx, req)
	if err != nil {
		return fmt.Errorf("unable to stop instance: %w", err)
	}

	if err = WaitOp(op, ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}

	return nil
}

func (g *GcpCli) StartInstance(ctx context.Context, instance string) error {
	req := &computepb.StartInstanceRequest{
		Instance: util.GetInstanceName(instance),
		Project:  g.cfg.ProjectId,
		Zone:     g.cfg.Zone,
	}

	op, err := g.client.Start(ctx, req)
	if err != nil {
		return fmt.Errorf("unable to start instance: %w", err)
	}

	if err = WaitOp(op, ctx); err != nil {
		return fmt.Errorf("unable to wait for the operation: %w", err)
	}

	return nil
}

func selectStartupScript(osType params.OSType) string {
	switch osType {
	case params.Windows:
		return windowsStartupScript
	case params.Linux:
		return linuxUserData
	default:
		return ""
	}
}

func generateBootDisk(diskSize int64, image, snapshot string, diskType string, customLabels map[string]string) []*computepb.AttachedDisk {
	disk := []*computepb.AttachedDisk{
		{
			Boot: proto.Bool(true),
			InitializeParams: &computepb.AttachedDiskInitializeParams{
				DiskSizeGb:     proto.Int64(diskSize),
				Labels:         customLabels,
				SourceImage:    proto.String(image),
				SourceSnapshot: proto.String(snapshot),
			},
			AutoDelete: proto.Bool(true),
		},
	}

	if diskType != "" {
		disk[0].InitializeParams.DiskType = proto.String(diskType)
	}

	if snapshot != "" {
		disk[0].InitializeParams.SourceImage = nil
	}

	return disk
}
