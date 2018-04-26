/*
Copyright 2018 The Kubernetes Authors.

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

package testrestmapper

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apimachinery"
	"k8s.io/apimachinery/pkg/apimachinery/registered"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

// TestOnlyStaticRESTMapper returns a union RESTMapper of all known types with priorities chosen in the following order:
//  1. legacy kube group preferred version, extensions preferred version, metrics perferred version, legacy
//     kube any version, extensions any version, metrics any version, all other groups alphabetical preferred version,
//     all other groups alphabetical.
func TestOnlyStaticRESTMapper(m *registered.APIRegistrationManager, scheme *runtime.Scheme, versionPatterns ...schema.GroupVersion) meta.RESTMapper {
	unionMapper := meta.MultiRESTMapper{}
	unionedGroups := sets.NewString()
	for _, enabledVersion := range m.RegisteredGroupVersions() {
		if !unionedGroups.Has(enabledVersion.Group) {
			unionedGroups.Insert(enabledVersion.Group)
			groupMeta := m.GroupOrDie(enabledVersion.Group)
			if groupMeta != nil {
				unionMapper = append(unionMapper, newRESTMapper(scheme, groupMeta))
			}
		}
	}

	if len(versionPatterns) != 0 {
		resourcePriority := []schema.GroupVersionResource{}
		kindPriority := []schema.GroupVersionKind{}
		for _, versionPriority := range versionPatterns {
			resourcePriority = append(resourcePriority, versionPriority.WithResource(meta.AnyResource))
			kindPriority = append(kindPriority, versionPriority.WithKind(meta.AnyKind))
		}

		return meta.PriorityRESTMapper{Delegate: unionMapper, ResourcePriority: resourcePriority, KindPriority: kindPriority}
	}

	prioritizedGroups := []string{"", "extensions", "metrics"}
	resourcePriority, kindPriority := prioritiesForGroups(m, prioritizedGroups...)

	prioritizedGroupsSet := sets.NewString(prioritizedGroups...)
	remainingGroups := sets.String{}
	for _, enabledVersion := range m.RegisteredGroupVersions() {
		if !prioritizedGroupsSet.Has(enabledVersion.Group) {
			remainingGroups.Insert(enabledVersion.Group)
		}
	}

	remainingResourcePriority, remainingKindPriority := prioritiesForGroups(m, remainingGroups.List()...)
	resourcePriority = append(resourcePriority, remainingResourcePriority...)
	kindPriority = append(kindPriority, remainingKindPriority...)

	return meta.PriorityRESTMapper{Delegate: unionMapper, ResourcePriority: resourcePriority, KindPriority: kindPriority}
}

// prioritiesForGroups returns the resource and kind priorities for a PriorityRESTMapper, preferring the preferred version of each group first,
// then any non-preferred version of the group second.
func prioritiesForGroups(m *registered.APIRegistrationManager, groups ...string) ([]schema.GroupVersionResource, []schema.GroupVersionKind) {
	resourcePriority := []schema.GroupVersionResource{}
	kindPriority := []schema.GroupVersionKind{}

	for _, group := range groups {
		availableVersions := m.RegisteredVersionsForGroup(group)
		if len(availableVersions) > 0 {
			resourcePriority = append(resourcePriority, availableVersions[0].WithResource(meta.AnyResource))
			kindPriority = append(kindPriority, availableVersions[0].WithKind(meta.AnyKind))
		}
	}
	for _, group := range groups {
		resourcePriority = append(resourcePriority, schema.GroupVersionResource{Group: group, Version: meta.AnyVersion, Resource: meta.AnyResource})
		kindPriority = append(kindPriority, schema.GroupVersionKind{Group: group, Version: meta.AnyVersion, Kind: meta.AnyKind})
	}

	return resourcePriority, kindPriority
}

func newRESTMapper(scheme *runtime.Scheme, groupMeta *apimachinery.GroupMeta) meta.RESTMapper {
	// the list of kinds that are scoped at the root of the api hierarchy
	// if a kind is not enumerated here, it is assumed to have a namespace scope
	rootScoped := sets.NewString()
	if groupMeta.RootScopedKinds != nil {
		rootScoped = groupMeta.RootScopedKinds
	}

	mapper := meta.NewDefaultRESTMapper(groupMeta.GroupVersions)
	for _, gv := range groupMeta.GroupVersions {
		for kind := range scheme.KnownTypes(gv) {
			if ignoredKinds.Has(kind) {
				continue
			}
			scope := meta.RESTScopeNamespace
			if rootScoped.Has(kind) {
				scope = meta.RESTScopeRoot
			}
			mapper.Add(gv.WithKind(kind), scope)
		}
	}

	return mapper
}

// hardcoded is good enough for the test we're running
var ignoredKinds = sets.NewString(
	"ListOptions",
	"DeleteOptions",
	"Status",
	"PodLogOptions",
	"PodExecOptions",
	"PodAttachOptions",
	"PodPortForwardOptions",
	"PodProxyOptions",
	"NodeProxyOptions",
	"ServiceProxyOptions",
)
