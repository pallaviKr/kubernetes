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

package kuberuntime

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api"
	runtimeApi "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

const (
	// containerNamePrefix is used to identify the containers/sandboxes on the node managed by kubelet
	containerNamePrefix = "k8s"

	// Taken from lmctfy https://github.com/google/lmctfy/blob/master/lmctfy/controllers/cpu_controller.cc
	minShares     = 2
	sharesPerCPU  = 1024
	milliCPUToCPU = 1000

	// 100000 is equivalent to 100ms
	quotaPeriod    = 100000
	minQuotaPeriod = 1000
)

// buildPodName creates a name which can be reversed to identify pod full name
// This function returns stable name, unique name and an unique id.
func buildPodName(podName, podNamespace, podUID string) (string, string, string) {
	return buildContainerName(podName, podNamespace, podUID, nil)
}

// parsePodName unpacks a pod full name, returning the pod name, namespace and uid
func parsePodName(name string) (string, string, string, error) {
	podName, podNamespace, podUID, _, _, err := parseContainerName(name)
	return podName, podNamespace, podUID, err
}

// buildContainerName creates a name which can be reversed to identify both full pod name and container name.
// This function returns stable name, unique name and an unique id.
func buildContainerName(podName, podNamespace, podUID string, container *api.Container) (string, string, string) {
	// Build name for pod sandbox if container is nil
	containerName := "POD"
	if container != nil {
		containerName = container.Name + "." + strconv.FormatUint(kubecontainer.HashContainer(container), 16)
	}

	stableName := fmt.Sprintf("%s_%s_%s_%s_%s",
		containerNamePrefix,
		containerName,
		podName,
		podNamespace,
		podUID,
	)
	UID := fmt.Sprintf("%08x", rand.Uint32())
	return stableName, fmt.Sprintf("%s_%s", stableName, UID), UID
}

// parseContainerName unpacks a container name, returning the pod full name and container name
func parseContainerName(name string) (podName, podNamespace, podUID, containerName string, hash uint64, err error) {
	// Some container runtimes appear to be appending '/' to names.
	name = strings.TrimPrefix(name, "/")
	parts := strings.Split(name, "_")
	if len(parts) == 0 || parts[0] != containerNamePrefix {
		err = fmt.Errorf("failed to parse container name %q into parts", name)
		return "", "", "", "", 0, err
	}
	if len(parts) < 6 {
		glog.Warningf("found a container with the %q prefix, but too few fields (%d): %q", containerNamePrefix, len(parts), name)
		err = fmt.Errorf("Container name %q has less parts than expected %v", name, parts)
		return "", "", "", "", 0, err
	}

	nameParts := strings.Split(parts[1], ".")
	containerName = nameParts[0]
	if len(nameParts) > 1 {
		hash, err = strconv.ParseUint(nameParts[1], 16, 32)
		if err != nil {
			glog.Warningf("invalid container hash %q in container %q", nameParts[1], name)
		}
	}

	return parts[2], parts[3], parts[4], containerName, hash, nil
}

// toRuntimeProtocol converts api.Protocol to runtimeApi.Protocol
func toRuntimeProtocol(protocol api.Protocol) runtimeApi.Protocol {
	switch protocol {
	case api.ProtocolTCP:
		return runtimeApi.Protocol_TCP
	case api.ProtocolUDP:
		return runtimeApi.Protocol_UDP
	}

	glog.Warningf("Unknown protocol %q: defaulting to TCP", protocol)
	return runtimeApi.Protocol_TCP
}

// milliCPUToShares converts milliCPU to CPU shares
func milliCPUToShares(milliCPU int64) int64 {
	if milliCPU == 0 {
		// Docker converts zero milliCPU to unset, which maps to kernel default
		// for unset: 1024. Return 2 here to really match kernel default for
		// zero milliCPU.
		return minShares
	}
	// Conceptually (milliCPU / milliCPUToCPU) * sharesPerCPU, but factored to improve rounding.
	shares := (milliCPU * sharesPerCPU) / milliCPUToCPU
	if shares < minShares {
		return minShares
	}
	return shares
}

// milliCPUToQuota converts milliCPU to CFS quota and period values
func milliCPUToQuota(milliCPU int64) (quota int64, period int64) {
	// CFS quota is measured in two values:
	//  - cfs_period_us=100ms (the amount of time to measure usage across)
	//  - cfs_quota=20ms (the amount of cpu time allowed to be used across a period)
	// so in the above example, you are limited to 20% of a single CPU
	// for multi-cpu environments, you just scale equivalent amounts

	if milliCPU == 0 {
		// take the default behavior from docker
		return
	}

	// we set the period to 100ms by default
	period = quotaPeriod

	// we then convert your milliCPU to a value normalized over a period
	quota = (milliCPU * quotaPeriod) / milliCPUToCPU

	// quota needs to be a minimum of 1ms.
	if quota < minQuotaPeriod {
		quota = minQuotaPeriod
	}

	return
}
