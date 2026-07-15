# Capacity-policy fork review

## 2026-07-15T11:21:06Z — adversarial base-to-HEAD review

### Scope and result

- Repository: `/Users/orierel/Desktop/garm-provider-gcp` only.
- Reviewed base: `a318f6358662056c7ce2d30ff70bb23a69c2e7db`.
- Corrected implementation head: `d7fd8df442bd4fcf5f75692b290bc96887d97d1d`.
- Result: PASS for the fork gates after the focused fixes below.
- No external repository or live GARM/GCE fleet reads or mutations were performed.

### Findings and focused commits

1. Regional request construction did not match the successful API proof: it repeated the base disk in `instanceProperties`, ignored the merged `network_id`, treated equivalent candidate-zone sets as different when ordered differently, silently dropped `display_device=true`, and leaked the zonal client if regional-client construction failed. Legacy runner-spec validation had also been broadened unintentionally. Fixed and test-pinned in `9f41d247a2ff6bff20f713443444cce89ee37414` (`fix: align regional placement requests`).
2. A capacity-looking timeout could advance after one empty exact-name lookup. The old test only covered an instance that was already visible and therefore did not prove duplicate prevention. Ambiguous timeouts, cancellations, and transport failures are now terminal after reconciliation; exact-name lookup is proved across all allowed zones. Fixed in `e3f929c4b2c389c9467c3d6ea8c1854a821f226d` (`fix: stop retries after ambiguous creates`).
3. Structured 403 errors whose text contained `not found` were misclassified as 404 and triggered legacy fallback lookup. Zoned IDs did not normalize the zone. GCE's real `X86_64`/`ARM64` disk values were returned as invalid GARM architectures, and missing disk architecture could panic. Fixed in `e4f3856db89abc4344ad16a14da0caaeed48de40` (`fix: preserve instance lookup semantics`).
4. README/schema coverage was incomplete: most policy validation branches were unpinned, the documented schema had an unresolved `ServiceAccount` reference, omitted `pre_install_scripts` and `boot_disk_kms_key_name`, and misspelled three description keys. Fixed in `4e6ce3ad40e0114bc2c899f9bcf011d49317d19a` (`docs: complete capacity policy schema`). The README schema block parses with `jq -e`.
5. Placement classification used only rendered error strings. The pinned SDK's `apierror.APIError` can omit the underlying `googleapi.Error.Errors[].Reason`, causing both missed quota/capacity advances and false fallback from non-capacity messages. Structured reasons now take precedence; unstructured messages remain a compatibility fallback. Fixed in `3fcc0223486768bd471aa0f7c294eb1fb414513f` (`fix: classify structured placement errors`).
6. The tests asserted protobuf fields but not the JSON body actually marshaled by the regional REST client. The request now omits empty disk-source fields and a regression test uses the SDK's `protojson.MarshalOptions{AllowPartial:true}` wire path to pin count/minCount, exact name, ANY_SINGLE_ZONE, SPOT scheduling, GVNIC, service account, labels/metadata, ranked machine selection, architecture, and per-candidate image/disk overrides. Fixed in `ef916907f14879ff5d1937544d7649b0d2ca2589` (`fix: match bulk insert wire request`).
7. Backward compatibility lacked a complete zonal request assertion. Configured zone, pool flavor/image/disk, network/subnetwork, STANDARD scheduling omission, and bare-name behavior are now pinned in `d7fd8df442bd4fcf5f75692b290bc96887d97d1d` (`test: pin legacy placement compatibility`). Existing SPOT-only fallback tests remain green and non-capacity errors still stop after one insert.

### Gate evidence

- Ordered provisioning models and ranked candidates: `TestBuildPlacementAttemptsOrderingAndZoneCompatibility`, `TestBuildPlacementAttemptsTreatsCandidateZonesAsASet`, `TestCapacityErrorAdvancesProvisioningModel`, and the SDK wire-shape test.
- Candidate zone compatibility: candidate subsets are validated against policy zones, canonicalized in policy order, and exercised by the t2a/c4a-style advance test.
- Per-candidate image/disk overrides: protobuf and REST-wire tests pin image, disk type, disk size, architecture, and retained rank.
- Classification/fallback: table tests cover structured and unstructured capacity, quota, auth, permission, invalid machine/image/disk/network, malformed request, and ambiguous timeout. Structured non-capacity reasons override misleading message text.
- Quota boundary: `QUOTA_EXCEEDED` removes only the lowest-ranked remaining candidate, emits `gcp_capacity_policy_quota_advance`, and cannot cross into another provisioning model.
- Terminal aggregation: exhausted classified attempts include model, machine type, compatible zones, and the wrapped reason for every candidate.
- Ambiguous-create dedup: every create error performs exact-name reconciliation; a found instance is returned, while an absent ambiguous result is terminal and never issues another bulk insert.
- IDs and lifecycle: policy instances use `zone/name`; Get/Delete/Start/Stop accept zoned IDs; legacy bare Get/Delete first use the configured zone and then exact aggregated lookup. Listing emits zoned IDs only for policy-marked instances.
- Architecture: schema requires each declaration, validation rejects unsupported, mixed, or pool-mismatched architecture, selection disks carry the declared GCE architecture, and GCE output is normalized back to GARM `amd64`/`arm64`.
- Backward compatibility: without `capacity_policy`, creation remains zonal and uses the configured zone plus pool flavor/image and legacy provisioning fields. Policy-only validation is not applied to legacy construction.
- Generic implementation: the base-to-HEAD diff contains no environment-specific project ID, repository path, provider pin, bucket, automated-author attribution, or generated-by marker.
- Branch integrity: local `main`, `origin/main`, and `upstream/main` all remain `cb14121f47281a330e7271079c7a54f625ecfe3b`; work is only on `capacity-policy`.

### Command evidence

All Go commands used `go version go1.25.12 linux/amd64` from `golang:1.25` with `--platform linux/amd64`.

```text
$ gofmt -l .
42 paths, all under vendor/

$ find . -path ./vendor -prune -o -name '*.go' -print | <provider-owned gofmt -l check>
<no output; exit 0>

$ git diff --name-only a318f6358662056c7ce2d30ff70bb23a69c2e7db..HEAD -- '*.go' | <changed-file gofmt -l check>
<no output; exit 0>

$ go vet ./...
<no output; exit 0>

$ go test -mod=vendor ./...
?    github.com/cloudbase/garm-provider-gcp [no test files]
ok   github.com/cloudbase/garm-provider-gcp/config 0.042s
ok   github.com/cloudbase/garm-provider-gcp/internal/client 0.110s
ok   github.com/cloudbase/garm-provider-gcp/internal/spec 0.114s
ok   github.com/cloudbase/garm-provider-gcp/internal/util 0.075s
ok   github.com/cloudbase/garm-provider-gcp/provider 0.120s

$ go test -race -mod=vendor ./...
?    github.com/cloudbase/garm-provider-gcp [no test files]
ok   github.com/cloudbase/garm-provider-gcp/config 1.063s
ok   github.com/cloudbase/garm-provider-gcp/internal/client 1.226s
ok   github.com/cloudbase/garm-provider-gcp/internal/spec 1.317s
ok   github.com/cloudbase/garm-provider-gcp/internal/util 1.114s
ok   github.com/cloudbase/garm-provider-gcp/provider 1.236s

$ git diff --quiet a318f6358662056c7ce2d30ff70bb23a69c2e7db..HEAD -- vendor
exit 0
```

The supplied note said the literal formatter baseline contained 43 vendor files. The actual pinned base and current unchanged vendor tree produce 42 paths under Go 1.25.12. No vendor file was formatted or changed; the empty base-to-HEAD vendor diff is the binding hygiene proof.
