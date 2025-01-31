# How to cut a CAPI provider release

For `minor` releases you need to do the following pre-release steps:

1. Add the new release with the corresponding CAPI contract to [the metadata file](../metadata.yaml)
2. Update the integration test files with the latest minor version:
    * https://github.com/canonical/cluster-api-k8s/blob/91e4fe70dfb7aaa357396d62c84f0d60de45595f/test/e2e/config/ck8s-docker.yaml#66
    * https://github.com/canonical/cluster-api-k8s/blob/91e4fe70dfb7aaa357396d62c84f0d60de45595f/test/e2e/config/ck8s-docker.yaml#77
    * https://github.com/canonical/cluster-api-k8s/blob/91e4fe70dfb7aaa357396d62c84f0d60de45595f/test/e2e/config/ck8s-aws.yaml#L64
    * https://github.com/canonical/cluster-api-k8s/blob/91e4fe70dfb7aaa357396d62c84f0d60de45595f/test/e2e/config/ck8s-aws.yaml#L74

Now, for `minor` and `patch` releases alike, you only need to create a new tag `vX.Y.ZZ` or `vX.Y.ZZ-rcABC` for release candidates.
The [Github workflow](https://github.com/canonical/cluster-api-k8s/blob/main/.github/workflows/release.yaml#L7) will then automatically create the release packages and add a new release to Github.
