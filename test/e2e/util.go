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
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"time"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type k8sdMember struct {
	Name          string `yaml:"name"`
	Address       string `yaml:"address"`
	ClusterRole   string `yaml:"cluster-role"`
	DatastoreRole string `yaml:"datastore-role"`
}

type k8sdClusterStatus struct {
	Members []k8sdMember `yaml:"members"`
}

// getK8sdClusterMembers returns the member list of the K8sd cluster.
func getK8sdClusterMembers(ctx context.Context, clientset *kubernetes.Clientset) ([]k8sdMember, error) {
	cmd := []string{"/snap/k8s/current/bin/k8s", "status", "--output-format", "yaml"}
	output, err := runCommandOnControlPlane(ctx, clientset, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to run command on control plane node: %w", err)
	}

	clusterStatus := k8sdClusterStatus{}
	err = yaml.Unmarshal([]byte(output), &clusterStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml data: %w", err)
	}

	return clusterStatus.Members, nil
}

// runCommandOnControlPlane will run the given command on a control plane node through a pod.
func runCommandOnControlPlane(ctx context.Context, clientset *kubernetes.Clientset, cmd []string) (string, error) {
	podName := fmt.Sprintf("node-exec-%d", rand.Int31())

	klog.Infof("Creating pod %s...", podName)
	if err := createControlPlanePod(ctx, clientset, "default", podName, cmd); err != nil {
		return "", fmt.Errorf("failed to create pod: %w", err)
	}

	defer func() {
		fmt.Println("Deleting pod...")
		if err := deletePod(ctx, clientset, "default", podName); err != nil {
			klog.Errorf("Failed to delete pod: %v", err)
		}
	}()

	klog.Infof("Waiting for pod %s to complete...", podName)
	if err := waitForPodCompletion(ctx, clientset, "default", podName); err != nil {
		return "", fmt.Errorf("pod did not complete execution: %w", err)
	}

	logs, err := getPodLogs(ctx, clientset, "default", podName)
	if err != nil {
		return "", fmt.Errorf("failed to get pod logs: %w", err)
	}

	return logs, nil
}

// createControlPlanePod creates a temporary privileged pod on a control plane node to execute a command on it.
func createControlPlanePod(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string, cmd []string) error {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: v1.PodSpec{
			NodeSelector: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
			Containers: []v1.Container{
				{
					Name:    "exec-container",
					Image:   "busybox",
					Command: cmd,
					// Needed in order to use k8s snap binary.
					Env: []v1.EnvVar{
						{Name: "SNAP", Value: "/snap/k8s/current"},
						{Name: "SNAP_COMMON", Value: "/var/snap/k8s/common"},
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "host-volume",
							MountPath: "/",
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "host-volume",
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{
							Path: "/",
						},
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
		},
	}

	_, err := clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	return err
}

// waitForPodCompletion waits for the pod to finish executing (either success of failure).
func waitForPodCompletion(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string) error {
	return wait.PollImmediate(2*time.Second, 60*time.Second, func() (bool, error) {
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed, nil
	})
}

// getPodLogs fetches the logs from the given pod.
func getPodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string) (string, error) {
	podLogRequest := clientset.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{})
	logStream, err := podLogRequest.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer logStream.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(logStream)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func deletePod(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string) error {
	return clientset.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
}
