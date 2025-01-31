//go:build e2e
// +build e2e

/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
    "context"
    "crypto/rand"
    "crypto/x509"
    "crypto/x509/pkix"
    "crypto/rsa"
    "encoding/pem"
    "fmt"
    "math/big"
    "path/filepath"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/utils/pointer"
    "sigs.k8s.io/cluster-api/test/framework/clusterctl"
    "sigs.k8s.io/cluster-api/util"
)

var _ = Describe("Intermediate CA", func() {
    var (
        ctx                    = context.TODO()
        specName               = "workload-cluster-intermediate-refresh"
        namespace              *corev1.Namespace
        cancelWatches          context.CancelFunc
        result                 *ApplyClusterTemplateAndWaitResult
        clusterName            string
        clusterctlLogFolder    string
        infrastructureProvider string
    )

    BeforeEach(func() {
        Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))

        clusterName = fmt.Sprintf("capick8s-certificate-refresh-%s", util.RandomString(6))
        infrastructureProvider = clusterctl.DefaultInfrastructureProvider

        // Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
        namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)
        result = new(ApplyClusterTemplateAndWaitResult)
        clusterctlLogFolder = filepath.Join(artifactFolder, "clusters", bootstrapClusterProxy.GetName())
    })

    AfterEach(func() {
        cleanInput := cleanupInput{
            SpecName:        specName,
            Cluster:         result.Cluster,
            ClusterProxy:    bootstrapClusterProxy,
            Namespace:       namespace,
            CancelWatches:   cancelWatches,
            IntervalsGetter: e2eConfig.GetIntervals,
            SkipCleanup:     skipCleanup,
            ArtifactFolder:  artifactFolder,
        }

        dumpSpecResourcesAndCleanup(ctx, cleanInput)
    })

    Context("Creating a cluster", func() {
        // From CAPI's point of view, this might as well have been a self-signed
        // root CA but for the purpose of this test we'll use an intermediate CA.
        It("Should allow specifying intermediate CA certificates.", func() {
            By("Generate self-signed CA")
            notBefore := time.Now()
            // Expire after 1 year.
            notAfter := notBefore.AddDate(1, 0, 0)
            ca_cert := generateCertificate(
                pkix.Name{CommonName: "root"},
                notBefore, notAfter, true)
            ca_key, err := rsa.GenerateKey(rand.Reader, 2048)
            Expect(err).ShouldNot(HaveOccurred())

            By("Generate intermediate CA")
            intermediate_template := generateCertificate(
                  pkix.Name{CommonName: "kubernetes"}, notBefore, notAfter, true)
            intermediate_cert, intermediate_key := signCertificate(
                intermediate_template, ca_cert, ca_key,
            )

            By("Create CA secrets")
            mgmtClient := bootstrapClusterProxy.GetClientSet()

            // Create CA cert secret
            secret := corev1.Secret{
                TypeMeta: metav1.TypeMeta{
                    Kind:       "Secret",
                    APIVersion: "v1",
                },
                ObjectMeta: metav1.ObjectMeta{
                    Name: clusterName + "-ca",
                    Namespace: namespace.Name,
                },
                Data: map[string][]byte{
                    "tls.crt": []byte(intermediate_cert),
                    "tls.key": []byte(intermediate_key),
                },
                Type: "opaque",
            }

            _, err = mgmtClient.CoreV1().Secrets(namespace.Name).Create(
                ctx, &secret, metav1.CreateOptions{})
            Expect(err).ShouldNot(HaveOccurred())

            // Create client CA cert secret
            secret.Name = clusterName + "-cca"
            _, err = mgmtClient.CoreV1().Secrets(namespace.Name).Create(
                ctx, &secret, metav1.CreateOptions{})
            Expect(err).ShouldNot(HaveOccurred())

            By("Creating a workload cluster")
            ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
                ClusterProxy: bootstrapClusterProxy,
                ConfigCluster: clusterctl.ConfigClusterInput{
                    LogFolder:                clusterctlLogFolder,
                    ClusterctlConfigPath:     clusterctlConfigPath,
                    KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
                    InfrastructureProvider:   infrastructureProvider,
                    Namespace:                namespace.Name,
                    ClusterName:              clusterName,
                    KubernetesVersion:        e2eConfig.GetVariable(KubernetesVersion),
                    ControlPlaneMachineCount: pointer.Int64Ptr(1),
                    WorkerMachineCount:       pointer.Int64Ptr(3),
                },
                WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
                WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
                WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
            }, result)
        })
    })
})

func generateCertificate(subject pkix.Name, notBefore, notAfter time.Time, ca bool) *x509.Certificate {
    serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
    serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
    Expect(err).ShouldNot(HaveOccurred())

    cert := &x509.Certificate{
        SerialNumber:          serialNumber,
        Subject:               subject,
        NotBefore:             notBefore,
        NotAfter:              notAfter,
        BasicConstraintsValid: true,
        ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
    }
    if ca {
        cert.IsCA = true
        cert.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
    } else {
        cert.IsCA = false
        cert.KeyUsage = x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment | x509.KeyUsageDigitalSignature
    }

    return cert
}

func signCertificate(certificate *x509.Certificate, parent *x509.Certificate, signerPrivateKey any) (string, string) {
    key, err := rsa.GenerateKey(rand.Reader, 2048)
    Expect(err).ShouldNot(HaveOccurred())

    keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
    Expect(err).ShouldNot(HaveOccurred())

    derBytes, err := x509.CreateCertificate(rand.Reader, certificate, parent, &key.PublicKey, signerPrivateKey)
    Expect(err).ShouldNot(HaveOccurred())

    crtPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
    Expect(err).ShouldNot(HaveOccurred())

    return string(crtPEM), string(keyPEM)
}

