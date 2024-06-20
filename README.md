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
        "disksize": {
            "type": "integer",
            "description": "The size of the root disk in GB. Default is 127 GB."
        },
        "network_id": {
            "type": "string",
            "description": "The name of the network attached to the instance."
        },
        "subnet_id": {
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
        "runner_install_template": {
            "type": "string",
            "description": "This option can be used to override the default runner install template. If used, the caller is responsible for the correctness of the template as well as the suitability of the template for the target OS. Use the extra_context extra spec if your template has variables in it that need to be expanded."
        },
        "extra_context": {
            "type": "object",
            "description": "Extra context that will be passed to the runner_install_template.",
            "additionalProperties": {
                "type": "string"
            }
        }
    },
    "additionalProperties": false
}
```

An example of extra specs json would look like this:

```bash
{
    "disksize": 255,
    "network_id": "projects/garm-testing/global/networks/garm-2",
    "subnet_id": "projects/garm-testing/regions/europe-west1/subnetworks/garm",
    "nic_type": "VIRTIO_NET",
    "custom_labels": {"environment":"production","project":"myproject"},
    "network_tags": ["web-server", "production"],
    "source_snapshot": "projects/garm-testing/global/snapshots/garm-snapshot",
    "ssh_keys": ["username1:ssh_key1", "username2:ssh_key2"]
}
```

**NOTE**: The `custom_labels` and `network_tags` must meet the [GCP requirements for labels](https://cloud.google.com/compute/docs/labeling-resources#requirements) and the [GCP requirements for network tags](https://cloud.google.com/vpc/docs/add-remove-network-tags#restrictions)!

**NOTE**: The `ssh_keys` add the option to [connect to an instance via SSH](https://cloud.google.com/compute/docs/instances/ssh) (either Linux or Windows). After you added the key as `username:ssh_public_key`, you can use the `private_key` to connect to the Linux/Windows instance via `ssh -i private_rsa username@instance_ip`. For **Windows** instances, the provider installs on the instance `google-compute-engine-ssh` and `enables ssh` if a `ssh_key` is added to extra-specs.

To set it on an existing pool, simply run:

```bash
garm-cli pool update --extra-specs='{"disksize" : 100}' <POOL_ID>
```

You can also set a spec when creating a new pool, using the same flag.

Workers in that pool will be created taking into account the specs you set on the pool.
