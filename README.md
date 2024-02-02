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
CredentialsFile = "/home/ubuntu/service-account-key.json"
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

Here is an example for a Linux pool that uses the image specified by its image name:

**NOTE**: The provider supports only **UBUNTU** and **DEBIAN** images for Linux pools at the moment.

```bash
garm-cli pool create \
    --os-type linux \
    --os-arch amd64 \
    --enabled=true \
    --flavor e2-medium \
    --image  projects/debian-cloud/global/images/debian-11-bullseye-v20240110 \
    --min-idle-runners 0 \
    --repo eb3f78b6-d667-4717-97c4-7aa1f3852138 \
    --tags gcp,linux \
    --provider-name gcp
```

Always find a recent image to use. For example, to see available Windows server 2022 images, run something like `gcloud compute images list --filter windows-2022` or just search [here](https://console.cloud.google.com/compute/images).

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
        }
    }
}
```

An example of extra specs json would look like this:

```bash
{
    "disksize": 255,
    "network_id": "projects/garm-testing/global/networks/garm-2",
    "subnet_id": "projects/garm-testing/regions/europe-west1/subnetworks/garm",
    "nic_type": "VIRTIO_NET"
}
```

To set it on an existing pool, simply run:

```bash
garm-cli pool update --extra-specs='{"disksize" : 100}' <POOL_ID>
```

You can also set a spec when creating a new pool, using the same flag.

Workers in that pool will be created taking into account the specs you set on the pool.