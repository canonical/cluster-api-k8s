# e2e test

The e2e test use the [Cluster API test framework](https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc) and use the [CAPD](https://github.com/kubernetes-sigs/cluster-api/tree/main/test/infrastructure/docker) as the infrastructure provider. Please make sure you have [Docker](https://docs.docker.com/install/) and [kind](https://kind.sigs.k8s.io/) installed.

Refer to the [Testing Cluster API](https://cluster-api.sigs.k8s.io/developer/testing) for more information.

## Run the e2e test

The e2e image will be built with tag `dev`. You should build the image first before running the test.

```shell
make docker-build-e2e   # should be run everytime you change the controller code
make test-e2e   # run all e2e tests
```

### Run a specific e2e test

To run a specific e2e test, such as `[PR-Blocking]`, use the `GINKGO_FOCUS` environment variable as shown below:

```shell
make GINKGO_FOCUS="\\[PR-Blocking\\]" test-e2e  # only run e2e test with `[PR-Blocking]` in its spec name
```

### Use an existing cluster as the management cluster

This is useful if you want to use a cluster managed by Tilt.

```shell
make USE_EXISTING_CLUSTER=true test-e2e
```

### Run e2e tests on AWS

To run the tests on AWS you will need to set the AWS_B64ENCODED_CREDENTIALS environment variable.

Then, you can run:

```shell
make E2E_INFRA=aws test-e2e
```

**Note**: The remediation tests do not pass on cloud providers. We suggest excluding them from the test run. See https://kubernetes.slack.com/archives/C8TSNPY4T/p1680525266510109.

### Running the tests with Tilt

This section explains how to run the E2E tests on AWS using a management cluster run by Tilt.

This section assumes you have *kind* and *Docker* installed. (See [Prerequisites](https://cluster-api.sigs.k8s.io/developer/tilt#prerequisites))

First, clone the upstream cluster-api and cluster-api-provider-aws repositories.
```shell
git clone https://github.com/kubernetes-sigs/cluster-api.git
git clone https://github.com/kubernetes-sigs/cluster-api-provider-aws.git
```

Next, you need to create a `tilt-settings.yaml` file inside the `cluster-api` directory.
The kustomize_substitutions you provide here are automatically applied to the *management cluster*.
```shell
default_registry: "ghcr.io/canonical/cluster-api-k8s"
provider_repos:
- ../cluster-api-k8s
- ../cluster-api-provider-aws
enable_providers:
- aws
- ck8s-bootstrap
- ck8s-control-plane
```

Tilt will know how to run the aws provider controllers because the `cluster-api-provider-aws` repository has a `tilt-provider.yaml` file at it's root. Canonical Kubernetes also provides this file at the root of the repository. The CK8s provider names, ck8s-bootstrap and ck8s-control-plane, are defined in CK8's `tilt-provider.yaml` file.

Next, you have to customize the variables that will be substituted into the cluster templates applied by the tests (these are under `test/e2e/data/infrastructure-aws`). You can customize the variables in the `test/e2e/config/ck8s-aws.yaml` file under the `variables` key.

Finally, in one terminal, go into the `cluster-api` directory and run `make tilt-up`. You should see a kind cluster be created, and finally a message indicating that Tilt is available at a certain address.

In a second terminal in the `cluster-api-k8s` directory, run `make USE_EXISTING_CLUSTER=true test-e2e`.

### Cleaning up after an e2e test

The test framework tries it's best to cleanup resources after a test suite, but it is possible that
cloud resources are left over. This can be very problematic especially if you run the tests multiple times
while iterating on development (see [Cluster API Book - Tear down](https://cluster-api.sigs.k8s.io/developer/e2e#tear-down)).

You can use a tool like [aws-nuke](https://github.com/eriksten/aws-nuke) to cleanup your AWS account after a test.

## Develop an e2e test

Refer to [Developing E2E tests](https://cluster-api.sigs.k8s.io/developer/e2e) for a complete guide for developing e2e tests.

A guide for developing a ck8s e2e test:

* Group test specs by scenarios (e.g., `create_test`, `node_scale_test`, `upgrade_test`). Create a new file under `test/e2e/` for new scenarios.
* If a different docker cluster template is needed, create one under `test/e2e/infrastructure-docker/` and link it in `test/e2e/config/ck8s-docker.yaml`.
* Define tunable variables in the cluster template as environment variables under `variables` in `test/e2e/config/ck8s-docker.yaml`. Enable necessary feature flags here as well (e.g., `EXP_CLUSTER_RESOURCE_SET: "true"`).
* If reusing a [cluster-api test spec](https://github.com/kubernetes-sigs/cluster-api/tree/main/test/e2e), note that they assume the use of `KubeadmControlPlane`. For customization, copy code into `test/e2e/helpers.go`.

## Troubleshooting

* [Cluster API with Docker - "too many open files".](https://cluster-api.sigs.k8s.io/user/troubleshooting.html?highlight=too%20many#cluster-api-with-docker----too-many-open-files)
