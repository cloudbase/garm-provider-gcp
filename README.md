# Garm External Provider For GCP

The GCP external provider allows [garm](https://github.com/cloudbase/garm) to create Linux and Windows runners on top of GCP virtual machines.

## Build

Clone the repo:

```bash
git clone https://github.com/cloudbase/garm-provider-gcp
```

Build the binary:

```bash
cd garm-provider-gcp
go build .
```

Copy the binary on the same system where garm is running, and [point to it in the config](https://github.com/cloudbase/garm/blob/main/doc/providers.md#the-external-provider).

## Configure

The config file for this external provider is a simple toml used to configure the GCP credentials it needs to spin up virtual machines.

```bash
project_id = "garm-testing"
zone = "europe-west1-d"
network_id = "projects/garm-testing/global/networks/garm"
subnetwork_id = "projects/garm-testing/regions/europe-west1/subnetworks/garm"
# The credentials file is optional.
# Leave this empty if you want to use the default credentials.
credentials_file = "/home/ubuntu/service-account-key.json"
external_ip_access = true
```

NOTE: If you want to pass in credentials by using the `GOOGLE_APPLICATION_CREDENTIALS` environment variable, you can leave the `credentials_file` field empty, but you must pass in the variable to GARM, then in the GARM config file, you must specify that the `GOOGLE_APPLICATION_CREDENTIALS` is safe to pass to the provider by setting the `environment_variables` field to `["GOOGLE_APPLICATION_CREDENTIALS"]`:

```toml
[[provider]]
  name = "gcp"
  provider_type = "external"
  description = "gcp provider"
  [provider.external]
    provider_executable = "/opt/garm/providers.d/garm-provider-gcp"
    config_file = "/etc/garm/garm-provider-gcp.toml"
    # This is needed if you want GARM to pass this along to the provider.
    environment_variables = ["GOOGLE_APPLICATION_CREDENTIALS"]
```

## Creating a pool

After you [add it to garm as an external provider](https://github.com/cloudbase/garm/blob/main/doc/providers.md#the-external-provider), you need to create a pool that uses it. Assuming you named your external provider as ```gcp``` in the garm config, the following command should create a new pool:

```bash
garm-cli pool create \
    --os-type windows \
    --os-arch amd64 \
    --enabled=true \
    --flavor e2-medium \
    --image  projects/windows-cloud/global/images/family/windows-2022 \
    --min-idle-runners 0 \
    --repo 26ae13a1-13e9-47ec-92c9-1526084684cf \
    --tags gcp,windows \
    --provider-name gcp
```

This will create a new Windows runner pool for the repo with ID `26ae13a1-13e9-47ec-92c9-1526084684cf` on GCP, using the image specified by its family name `projects/windows-cloud/global/images/family/windows-2022` and instance type `e2-medium`. You can, of course, tweak the values in the above command to suit your needs.

**NOTE**: If you want to use a custom image that you created, specify the image name in the following format: `projects/my_project/global/images/my-custom-image`

Always find a recent image to use. For example, to see available Windows server 2022 images, run something like `gcloud compute images list --filter windows-2022` or just search [here](https://console.cloud.google.com/compute/images).

Linux pools support **ONLY** images with **CLOUD-INIT** already installed. Before using a linux pool, you must be sure that the image has cloud-init installed. Here is an example for a Linux pool that uses a custom image with cloud-init specified by its image name:

```bash
garm-cli pool create \
    --os-type linux \
    --os-arch amd64 \
    --enabled=true \
    --flavor e2-medium \
    --image  projects/garm-testing-424210/global/images/debian-cloud-init \
    --min-idle-runners 0 \
    --repo eb3f78b6-d667-4717-97c4-7aa1f3852138 \
    --tags gcp,linux \
    --provider-name gcp
```

**NOTE:** In order to [create a custom image](https://cloud.google.com/compute/docs/images/create-custom#create_image) with cloud-init, you have to *create an instance* with your desired Linux OS. Then connect to that instance and install `cloud-init`. After the install is finished, you can stop that instance and from the `Disk` of that instance, create a custom image. As example, if you use `projects/debian-cloud/global/images/debian-12-bookworm-v20240617`, you can install cloud-init on the instance like this:

```
sudo apt update && sudo apt install -y cloud-init
```

## Tweaking the provider

Garm supports sending opaque json encoded configs to the IaaS providers it hooks into. This allows the providers to implement some very provider specific functionality that doesn't necessarily translate well to other providers. Features that may exists on GCP, may not exist on Azure or AWS and vice versa.

To this end, this provider supports the following extra specs schema:

```bash
{
    "$schema": "http://cloudbase.it/garm-provider-gcp/schemas/extra_specs#",
    "type": "object",
    "description": "Schema defining supported extra specs for the Garm GCP Provider",
    "properties": {
        "capacity_policy": {
            "type": "object",
            "description": "Optional ordered regional capacity policy. When omitted the provider uses the configured zone and pool flavor.",
            "properties": {
                "zones": {
                    "type": "array",
                    "description": "Ordered Compute Engine zones allowed for regional placement.",
                    "items": {"type": "string"}
                },
                "candidates": {
                    "type": "array",
                    "description": "Ordered machine type candidates. Earlier entries have a lower GCE flexibility rank.",
                    "items": {
                        "type": "object",
                        "properties": {
                            "machine_type": {"type": "string", "description": "Compute Engine machine type name without a zone or URL."},
                            "architecture": {"type": "string", "description": "Runner CPU architecture. Supported values are amd64 and arm64."},
                            "zones": {"type": "array", "description": "Optional compatible subset of the policy zones. Empty means every policy zone.", "items": {"type": "string"}},
                            "image": {"type": "string", "description": "Optional source image override. The pool image is used when omitted."},
                            "disk_type": {"type": "string", "description": "Optional boot disk type override. The pool disktype is used when omitted."},
                            "disk_size": {"type": "integer", "description": "Optional boot disk size override in GB. Zero uses the pool disksize."}
                        },
                        "additionalProperties": false,
                        "required": ["machine_type", "architecture"]
                    }
                },
                "provisioning_models": {
                    "type": "array",
                    "description": "Ordered Compute Engine provisioning models. Supported values are SPOT and STANDARD.",
                    "items": {"type": "string"}
                }
            },
            "additionalProperties": false,
            "required": ["zones", "candidates", "provisioning_models"]
        },
        "provisioning_model": {
            "type": "string",
            "description": "Compute Engine provisioning model for legacy zonal placement. Supported values are STANDARD and SPOT."
        },
        "fallback_to_standard": {
            "type": "boolean",
            "description": "Retry a legacy zonal SPOT create as STANDARD only when the SPOT failure is a recognized capacity error."
        },
        "display_device": {
            "type": "boolean",
            "description": "Enable the display device on a legacy zonal VM. This field cannot be combined with capacity_policy because regional bulk insert does not expose display-device settings."
        },
        "disksize": {
            "type": "integer",
            "description": "The size of the root disk in GB. Default is 127 GB."
        },
        "disktype": {
            "type": "string",
            "description": "The type of the disk. Default is pd-standard."
        },
        "network_id": {
            "type": "string",
            "description": "The name of the network attached to the instance."
        },
        "subnetwork_id": {
            "type": "string",
            "description": "The name of the subnetwork attached to the instance."
        },
        "nic_type": {
            "type": "string",
            "description": "The type of NIC attached to the instance. Default is VIRTIO_NET."
        },
        "custom_labels":{
            "type": "object",
            "description": "Custom labels to be attached to the instance. Each label is a key-value pair where both key and value are strings.",
            "additionalProperties": {
                "type": "string"
            }
        },
        "network_tags": {
            "type": "array",
            "description": "A list of network tags to be attached to the instance.",
            "items": {
                "type": "string"
            }
        },
        "service_accounts": {
            "type": "array",
            "description": "A list of service accounts to be attached to the instance",
            "items": {
                "$ref": "#/$defs/ServiceAccount"
            }
        },
        "source_snapshot": {
            "type": "string",
            "description": "The source snapshot to create this disk."
        },
        "ssh_keys": {
            "type": "array",
            "description": "A list of SSH keys to be added to the instance. The format is USERNAME:SSH_KEY",
            "items": {
                "type": "string"
            }
        },
        "enable_boot_debug": {
            "type": "boolean",
            "description": "Enable boot debug on the VM."
        },
        "disable_updates": {
            "type": "boolean",
            "description": "Disable OS updates on boot."
        },
        "enable_secure_boot": {
            "type": "boolean",
            "description": "Enable Secure Boot on the VM. Requires a Shielded VM compatible image."
        },
        "enable_vtpm": {
            "type": "boolean",
            "description": "Enable virtual Trusted Platform Module (vTPM) on the VM."
        },
        "enable_integrity_monitoring": {
            "type": "boolean",
            "description": "Enable integrity monitoring on the VM."
        },
        "boot_disk_kms_key_name": {
            "type": "string",
            "description": "The Cloud KMS key used to encrypt the boot disk."
        },
        "runner_install_template": {
            "type": "string",
            "description": "This option can be used to override the default runner install template. If used, the caller is responsible for the correctness of the template as well as the suitability of the template for the target OS. Use the extra_context extra spec if your template has variables in it that need to be expanded."
        },
        "pre_install_scripts": {
            "type": "object",
            "description": "Base64-encoded scripts run in key order before the runner install script.",
            "additionalProperties": {
                "type": "string"
            }
        },
        "extra_context": {
            "type": "object",
            "description": "Extra context that will be passed to the runner_install_template.",
            "additionalProperties": {
                "type": "string"
            }
        }
    },
    "$defs": {
        "ServiceAccount": {
            "type": "object",
            "properties": {
                "email": {"type": "string"},
                "scopes": {"type": "array", "items": {"type": "string"}}
            },
            "additionalProperties": false
        }
    },
    "additionalProperties": false
}
```

An example of extra specs json would look like this:

```bash
{
    "display_device": true,
    "disksize": 255,
    "disktype": "projects/garm-testing/zones/europe-west1/diskTypes/pd-ssd",
    "network_id": "projects/garm-testing/global/networks/garm-2",
    "subnetwork_id": "projects/garm-testing/regions/europe-west1/subnetworks/garm",
    "nic_type": "VIRTIO_NET",
    "custom_labels": {"environment":"production","project":"myproject"},
    "network_tags": ["web-server", "production"],
    "service_accounts": [{"email":"email@email.com", "scopes":["https://www.googleapis.com/auth/devstorage.read_only", "https://www.googleapis.com/auth/logging.write"]}],
    "source_snapshot": "projects/garm-testing/global/snapshots/garm-snapshot",
    "ssh_keys": ["username1:ssh_key1", "username2:ssh_key2"]
}
```

**NOTE**: Using the `service_accounts` extra specs when creating instances **introduces certain risks that must be carefully managed**. **Service accounts** grant access to specific resources, and if improperly configured, they can expose sensitive data or allow unauthorized actions. Misconfigured permissions or overly broad scopes can lead to privilege escalation, enabling attackers or unintended users to access critical resources. It's essential to follow the principle of least privilege, ensuring that service accounts only have the necessary permissions for their intended tasks. Regular audits and proper key management are also crucial to safeguard access and prevent potential security vulnerabilities.

**NOTE**: The `custom_labels` and `network_tags` must meet the [GCP requirements for labels](https://cloud.google.com/compute/docs/labeling-resources#requirements) and the [GCP requirements for network tags](https://cloud.google.com/vpc/docs/add-remove-network-tags#restrictions)!

**NOTE**: The `ssh_keys` add the option to [connect to an instance via SSH](https://cloud.google.com/compute/docs/instances/ssh) (either Linux or Windows). After you added the key as `username:ssh_public_key`, you can use the `private_key` to connect to the Linux/Windows instance via `ssh -i private_rsa username@instance_ip`. For **Windows** instances, the provider installs on the instance `google-compute-engine-ssh` and `enables ssh` if a `ssh_key` is added to extra-specs.

To set it on an existing pool, simply run:

```bash
garm-cli pool update --extra-specs='{"disksize" : 100}' <POOL_ID>
```

You can also set a spec when creating a new pool, using the same flag.

Workers in that pool will be created taking into account the specs you set on the pool.

### Regional capacity policies

Setting `capacity_policy` switches creation from a zonal insert to a regional bulk insert with `ANY_SINGLE_ZONE`. Compute Engine chooses a zone from `zones` and uses the candidate array as a ranked instance flexibility policy. Every flexibility selection carries its own boot disk, including inherited pool values and any candidate overrides. The provider tries provisioning models in the declared order. It advances only after recognized capacity failures; quota exhaustion advances to the next ranked candidate with the `gcp_capacity_policy_quota_advance` log marker and never advances to a different provisioning model. Authentication, permission, malformed configuration, ambiguous transport failures, and invalid machine, image, disk, or network errors stop immediately.

Every candidate declares `architecture`, and all candidates must use the same architecture as the pool. Candidate `zones` can narrow placement for machine families that are not available in every policy zone; they are treated as a set and emitted in the policy's zone order. Consecutive candidates with the same compatible zone set are submitted together in rank order. Candidate image, disk type, and disk size values override the pool values only for that selection.

For example:

```json
{
  "capacity_policy": {
    "zones": ["us-central1-a", "us-central1-b", "us-central1-c"],
    "candidates": [
      {
        "machine_type": "t2a-standard-2",
        "architecture": "arm64",
        "zones": ["us-central1-a", "us-central1-b"]
      },
      {
        "machine_type": "c4a-standard-2",
        "architecture": "arm64",
        "image": "projects/example/global/images/runner-arm64",
        "disk_type": "hyperdisk-balanced",
        "disk_size": 150
      }
    ],
    "provisioning_models": ["SPOT", "STANDARD"]
  }
}
```

`capacity_policy` cannot be combined with the legacy `provisioning_model`, `fallback_to_standard`, or `display_device=true` fields. When `capacity_policy` is absent, the configured provider zone, the pool flavor and image, and the legacy provisioning fields retain their existing behavior.

Policy-created instances return provider IDs in `zone/name` form while the Compute Engine instance name remains unchanged. Get and delete accept both zoned IDs and legacy bare names, allowing existing runners to be reaped during migration. Before a regional create, the provider searches every policy zone for the exact name and reuses a matching policy instance only when its labels and policy marker match the requested runner. After any create error it searches the attempted zones again; timeout, cancellation, and ambiguous transport reconciliation uses a separate bounded read context, so caller cancellation cannot skip the deduplication check. If no VM is visible after an ambiguous result, the provider stops instead of issuing another create. If all eligible candidates fail with classified capacity or quota errors, the returned error includes the model, machine type, zones, and reason for every attempted candidate.
