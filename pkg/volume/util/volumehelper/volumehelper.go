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

// Package volumehelper contains consts and helper methods used by various
// volume components (attach/detach controller, kubelet, etc.).
package volumehelper

import (
	"fmt"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/kubernetes/pkg/volume/util/types"
)

const (
	// ControllerManagedAttachAnnotation is the key of the annotation on Node
	// objects that indicates attach/detach operations for the node should be
	// managed by the attach/detach controller
	ControllerManagedAttachAnnotation string = "volumes.kubernetes.io/controller-managed-attach-detach"

	// VolumeGidAnnotationKey is the of the annotation on the PersistentVolume
	// object that specifies a supplemental GID.
	VolumeGidAnnotationKey = "pv.beta.kubernetes.io/gid"
)

// GetUniquePodName returns a unique identifier to reference a pod by
func GetUniquePodName(pod *api.Pod) types.UniquePodName {
	return types.UniquePodName(pod.UID)
}

// GetUniqueVolumeName returns a unique name representing the volume/plugin.
// Caller should ensure that volumeName is a name/ID uniquely identifying the
// actual backing device, directory, path, etc. for a particular volume.
// The returned name can be used to uniquely reference the volume, for example,
// to prevent operations (attach/detach or mount/unmount) from being triggered
// on the same volume.
func GetUniqueVolumeName(pluginName, volumeName string) api.UniqueVolumeName {
	return api.UniqueVolumeName(fmt.Sprintf("%s/%s", pluginName, volumeName))
}

// GetUniqueVolumeNameForNonAttachableVolume returns the unique volume name
// for a non-attachable volume.
func GetUniqueVolumeNameForNonAttachableVolume(
	podName types.UniquePodName, volumePlugin volume.VolumePlugin, volumeSpec *volume.Spec) api.UniqueVolumeName {
	return api.UniqueVolumeName(
		fmt.Sprintf("%s/%v-%s", volumePlugin.GetPluginName(), podName, volumeSpec.Name()))
}

// GetUniqueVolumeNameFromSpec uses the given VolumePlugin to generate a unique
// name representing the volume defined in the specified volume spec.
// This returned name can be used to uniquely reference the actual backing
// device, directory, path, etc. referenced by the given volumeSpec.
// If the given plugin does not support the volume spec, this returns an error.
func GetUniqueVolumeNameFromSpec(
	volumePlugin volume.VolumePlugin,
	volumeSpec *volume.Spec) (api.UniqueVolumeName, error) {
	if volumePlugin == nil {
		return "", fmt.Errorf(
			"volumePlugin should not be nil. volumeSpec.Name=%q",
			volumeSpec.Name())
	}

	volumeName, err := volumePlugin.GetVolumeName(volumeSpec)
	if err != nil || volumeName == "" {
		return "", fmt.Errorf(
			"failed to GetVolumeName from volumePlugin for volumeSpec %q err=%v",
			volumeSpec.Name(),
			err)
	}

	return GetUniqueVolumeName(
			volumePlugin.GetPluginName(),
			volumeName),
		nil
}

// PostEventToPersistentVolumeClaim posts an event to the given PersistentVolumeClaim
// API object with the given message and event type.
func PostEventToPersistentVolumeClaim(
	kubeClient internalclientset.Interface,
	pvc *api.PersistentVolumeClaim,
	eventName string,
	message string,
	eventType string) error {
	timeStamp := unversioned.Now()
	name := fmt.Sprintf("%s-%s", pvc.Name, eventName)
	if event, err := kubeClient.Core().Events(pvc.Namespace).Get(name); err == nil {
		// event already exists, update the count and timeStamp
		event.Count++
		event.LastTimestamp = timeStamp
		_, updateErr := kubeClient.Core().Events(pvc.Namespace).Update(event)
		if updateErr != nil {
			return fmt.Errorf(
				"Failed to post event %q, err=%v",
				name,
				updateErr)
		}
	} else {
		ref, refErr := api.GetReference(runtime.Object(pvc))
		if refErr != nil {
			return fmt.Errorf(
				"Failed to GetReference from PersistentVolumeClaim %q, err=%v",
				pvc.Name,
				refErr)
		}
		event := &api.Event{
			ObjectMeta: api.ObjectMeta{
				Namespace: pvc.Namespace,
				Name:      name,
			},
			InvolvedObject: *ref,
			Message:        message,
			Source:         api.EventSource{Component: "controllermanager"},
			FirstTimestamp: timeStamp,
			LastTimestamp:  timeStamp,
			Count:          1,
			Type:           eventType,
		}
		_, createErr := kubeClient.Core().Events(pvc.Namespace).Create(event)
		if createErr != nil {
			return fmt.Errorf(
				"Failed to post event %q, err=%v",
				name,
				createErr)
		}
	}
	return nil
}
