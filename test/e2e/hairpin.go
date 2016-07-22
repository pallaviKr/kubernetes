/*
Copyright 2016 The Kubernetes Authors.

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
	"fmt"
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/util/intstr"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = framework.KubeDescribe("Hairpin Networking", func() {
	f := framework.NewDefaultFramework("hairpintest")

	var svcname = "hairpintest"

	It("should be able to contact a service served by the same pod", func() {

		By("Picking a node")
		nodes := framework.GetReadySchedulableNodesOrDie(f.Client)
		node := nodes.Items[0]

		By("Creating a webserver pod")
		podName := "hairpin-webserver"
		launchHairpinTestPod(f, podName, node.Name)
		defer f.Client.Pods(f.Namespace.Name).Delete(podName, nil)

		By(fmt.Sprintf("Creating a service named %q in namespace %q", svcname, f.Namespace.Name))
		svc, err := f.Client.Services(f.Namespace.Name).Create(&api.Service{
			ObjectMeta: api.ObjectMeta{
				Name: svcname,
				Labels: map[string]string{
					"name": svcname,
				},
			},
			Spec: api.ServiceSpec{
				Ports: []api.ServicePort{{
					Protocol:   "TCP",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				}},
				Selector: map[string]string{
					"name": podName,
				},
			},
		})
		if err != nil {
			framework.Failf("unable to create test service named [%s] %v", svc.Name, err)
		}

		// Clean up service
		defer func() {
			By("Cleaning up the service")
			if err = f.Client.Services(f.Namespace.Name).Delete(svc.Name); err != nil {
				framework.Failf("unable to delete svc %v: %v", svc.Name, err)
			}
		}()

		By("Checking that the webserver is accessible from inside the same pod")
		passed := false
		// apply a 1 minute observation period to ensure the pod and service have started
		timeout := time.Now().Add(1 * time.Minute)
		for i := 0; !passed && timeout.After(time.Now()); i++ {
			time.Sleep(2 * time.Second)
			_, err := framework.RunKubectl("exec", fmt.Sprintf("--namespace=%v", f.Namespace.Name), podName, "--", "wget", "-s", svcname+":8080")
			if err != nil {
				framework.Logf("Attempt %v: did not succeed. (error: '%v')", i, err)
				continue
			}
			passed = true
		}
		Expect(passed).Should(Equal(true))
	})
})

// launch a pod which has both a server and a client we can use to test connectivity to that server
// server code copied from LaunchWebserverPod()
func launchHairpinTestPod(f *framework.Framework, podName, nodeName string) {
	port := 8080
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				"name": podName,
			},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Name:    "client",
					Image:   "gcr.io/google_containers/busybox:1.24",
					Command: []string{"sleep", "3600"}, // we're going to exec the real test later
				},
				{
					Name:  "server",
					Image: "gcr.io/google_containers/porter:cd5cb5791ebaa8641955f0e8c2a9bed669b1eaab",
					Env:   []api.EnvVar{{Name: fmt.Sprintf("SERVE_PORT_%d", port), Value: "foo"}},
					Ports: []api.ContainerPort{{ContainerPort: int32(port)}},
				},
			},
			NodeName:      nodeName,
			RestartPolicy: api.RestartPolicyNever,
		},
	}
	podClient := f.Client.Pods(f.Namespace.Name)
	_, err := podClient.Create(pod)
	ExpectNoError(err)
	ExpectNoError(f.WaitForPodRunning(podName))
	return
}
