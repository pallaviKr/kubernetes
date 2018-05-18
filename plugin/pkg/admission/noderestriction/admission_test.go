/*
Copyright 2017 The Kubernetes Authors.

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

package noderestriction

import (
	"reflect"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/tools/cache"
	authenticationapi "k8s.io/kubernetes/pkg/apis/authentication"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/policy"
	"k8s.io/kubernetes/pkg/auth/nodeidentifier"
	"k8s.io/kubernetes/pkg/client/listers/core/internalversion"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

var (
	trEnabledFeature  = utilfeature.NewFeatureGate()
	trDisabledFeature = utilfeature.NewFeatureGate()
)

func init() {
	if err := trEnabledFeature.Add(map[utilfeature.Feature]utilfeature.FeatureSpec{features.TokenRequest: {Default: true}}); err != nil {
		panic(err)
	}
	if err := trDisabledFeature.Add(map[utilfeature.Feature]utilfeature.FeatureSpec{features.TokenRequest: {Default: false}}); err != nil {
		panic(err)
	}
}

func label(node *api.Node, labels ...map[string]string) *api.Node {
	// don't disrupt original
	node = node.DeepCopy()
	node.Labels = map[string]string{}
	for _, l := range labels {
		for k, v := range l {
			node.Labels[k] = v
		}
	}
	return node
}
func annotate(node *api.Node, annotations map[string]string) *api.Node {
	// don't disrupt original
	node = node.DeepCopy()
	node.Annotations = map[string]string{}
	for k, v := range annotations {
		node.Annotations[k] = v
	}
	return node
}

func makeTestPod(namespace, name, node string, mirror bool) *api.Pod {
	pod := &api.Pod{}
	pod.Namespace = namespace
	pod.UID = types.UID("pod-uid")
	pod.Name = name
	pod.Spec.NodeName = node
	if mirror {
		pod.Annotations = map[string]string{api.MirrorPodAnnotationKey: "true"}
	}
	return pod
}

func makeTestPodEviction(name string) *policy.Eviction {
	eviction := &policy.Eviction{}
	eviction.Name = name
	eviction.Namespace = "ns"
	return eviction
}

func makeTokenRequest(podname string, poduid types.UID) *authenticationapi.TokenRequest {
	tr := &authenticationapi.TokenRequest{
		Spec: authenticationapi.TokenRequestSpec{
			Audiences: []string{"foo"},
		},
	}
	if podname != "" {
		tr.Spec.BoundObjectRef = &authenticationapi.BoundObjectReference{
			Kind:       "Pod",
			APIVersion: "v1",
			Name:       podname,
			UID:        poduid,
		}
	}
	return tr
}

func Test_nodePlugin_Admit(t *testing.T) {
	var (
		mynode = &user.DefaultInfo{Name: "system:node:mynode", Groups: []string{"system:nodes"}}
		bob    = &user.DefaultInfo{Name: "bob"}

		allowedLabels = map[string]string{
			apis.LabelArch:              "value",
			apis.LabelOS:                "value",
			apis.LabelHostname:          "value",
			apis.LabelInstanceType:      "value",
			apis.LabelZoneFailureDomain: "value",
			apis.LabelZoneRegion:        "value",
		}
		allowedChangedLabels = map[string]string{
			apis.LabelArch:              "newvalue",
			apis.LabelOS:                "newvalue",
			apis.LabelHostname:          "newvalue",
			apis.LabelInstanceType:      "newvalue",
			apis.LabelZoneFailureDomain: "newvalue",
			apis.LabelZoneRegion:        "newvalue",
		}
		disallowedLabels = map[string]string{
			"custom": "value",
		}
		disallowedChangedLabels = map[string]string{
			"custom": "newvalue",
		}
		disallowedAdditionAnnotation = map[string]string{
			DisallowedLabelsAnnotationKey: `{"custom":"value"}`,
		}
		disallowedChangeAnnotation = map[string]string{
			DisallowedLabelsAnnotationKey: `{"custom":"newvalue"}`,
		}
		disallowedRemovalAnnotation = map[string]string{
			DisallowedLabelsAnnotationKey: `{"custom":""}`,
		}

		mynodeObjMeta    = metav1.ObjectMeta{Name: "mynode"}
		mynodeObj        = &api.Node{ObjectMeta: mynodeObjMeta}
		mynodeObjConfigA = &api.Node{ObjectMeta: mynodeObjMeta, Spec: api.NodeSpec{ConfigSource: &api.NodeConfigSource{
			ConfigMap: &api.ConfigMapNodeConfigSource{
				Name:             "foo",
				Namespace:        "bar",
				UID:              "fooUID",
				KubeletConfigKey: "kubelet",
			}}}}
		mynodeObjConfigB = &api.Node{ObjectMeta: mynodeObjMeta, Spec: api.NodeSpec{ConfigSource: &api.NodeConfigSource{
			ConfigMap: &api.ConfigMapNodeConfigSource{
				Name:             "qux",
				Namespace:        "bar",
				UID:              "quxUID",
				KubeletConfigKey: "kubelet",
			}}}}
		mynodeObjTaintA = &api.Node{ObjectMeta: mynodeObjMeta, Spec: api.NodeSpec{Taints: []api.Taint{{Key: "mykey", Value: "A"}}}}
		mynodeObjTaintB = &api.Node{ObjectMeta: mynodeObjMeta, Spec: api.NodeSpec{Taints: []api.Taint{{Key: "mykey", Value: "B"}}}}
		othernodeObj    = &api.Node{ObjectMeta: metav1.ObjectMeta{Name: "othernode"}}

		mymirrorpod      = makeTestPod("ns", "mymirrorpod", "mynode", true)
		othermirrorpod   = makeTestPod("ns", "othermirrorpod", "othernode", true)
		unboundmirrorpod = makeTestPod("ns", "unboundmirrorpod", "", true)
		mypod            = makeTestPod("ns", "mypod", "mynode", false)
		otherpod         = makeTestPod("ns", "otherpod", "othernode", false)
		unboundpod       = makeTestPod("ns", "unboundpod", "", false)
		unnamedpod       = makeTestPod("ns", "", "mynode", false)

		mymirrorpodEviction      = makeTestPodEviction("mymirrorpod")
		othermirrorpodEviction   = makeTestPodEviction("othermirrorpod")
		unboundmirrorpodEviction = makeTestPodEviction("unboundmirrorpod")
		mypodEviction            = makeTestPodEviction("mypod")
		otherpodEviction         = makeTestPodEviction("otherpod")
		unboundpodEviction       = makeTestPodEviction("unboundpod")
		unnamedEviction          = makeTestPodEviction("")

		configmapResource = api.Resource("configmap").WithVersion("v1")
		configmapKind     = api.Kind("ConfigMap").WithVersion("v1")

		podResource  = api.Resource("pods").WithVersion("v1")
		podKind      = api.Kind("Pod").WithVersion("v1")
		evictionKind = policy.Kind("Eviction").WithVersion("v1beta1")

		nodeResource = api.Resource("nodes").WithVersion("v1")
		nodeKind     = api.Kind("Node").WithVersion("v1")

		svcacctResource  = api.Resource("serviceaccounts").WithVersion("v1")
		tokenrequestKind = api.Kind("TokenRequest").WithVersion("v1")

		noExistingPodsIndex = cache.NewIndexer(cache.MetaNamespaceKeyFunc, nil)
		noExistingPods      = internalversion.NewPodLister(noExistingPodsIndex)

		existingPodsIndex = cache.NewIndexer(cache.MetaNamespaceKeyFunc, nil)
		existingPods      = internalversion.NewPodLister(existingPodsIndex)
	)

	existingPodsIndex.Add(mymirrorpod)
	existingPodsIndex.Add(othermirrorpod)
	existingPodsIndex.Add(unboundmirrorpod)
	existingPodsIndex.Add(mypod)
	existingPodsIndex.Add(otherpod)
	existingPodsIndex.Add(unboundpod)

	sapod := makeTestPod("ns", "mysapod", "mynode", true)
	sapod.Spec.ServiceAccountName = "foo"

	secretpod := makeTestPod("ns", "mysecretpod", "mynode", true)
	secretpod.Spec.Volumes = []api.Volume{{VolumeSource: api.VolumeSource{Secret: &api.SecretVolumeSource{SecretName: "foo"}}}}

	configmappod := makeTestPod("ns", "myconfigmappod", "mynode", true)
	configmappod.Spec.Volumes = []api.Volume{{VolumeSource: api.VolumeSource{ConfigMap: &api.ConfigMapVolumeSource{LocalObjectReference: api.LocalObjectReference{Name: "foo"}}}}}

	pvcpod := makeTestPod("ns", "mypvcpod", "mynode", true)
	pvcpod.Spec.Volumes = []api.Volume{{VolumeSource: api.VolumeSource{PersistentVolumeClaim: &api.PersistentVolumeClaimVolumeSource{ClaimName: "foo"}}}}

	tests := []struct {
		name       string
		podsGetter internalversion.PodLister
		attributes admission.Attributes
		features   utilfeature.FeatureGate
		err        string
		expected   runtime.Object
	}{
		// Mirror pods bound to us
		{
			name:       "allow creating a mirror pod bound to self",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mymirrorpod, nil, podKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "", admission.Create, mynode),
			err:        "",
		},
		{
			name:       "forbid update of mirror pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mymirrorpod, mymirrorpod, podKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "allow delete of mirror pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "", admission.Delete, mynode),
			err:        "",
		},
		{
			name:       "forbid create of mirror pod status bound to self",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mymirrorpod, nil, podKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "status", admission.Create, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "allow update of mirror pod status bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mymirrorpod, mymirrorpod, podKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "status", admission.Update, mynode),
			err:        "",
		},
		{
			name:       "forbid delete of mirror pod status bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "status", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "allow create of eviction for mirror pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mymirrorpodEviction, nil, evictionKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "",
		},
		{
			name:       "forbid update of eviction for mirror pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mymirrorpodEviction, nil, evictionKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "eviction", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of eviction for mirror pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mymirrorpodEviction, nil, evictionKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "eviction", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "allow create of unnamed eviction for mirror pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unnamedEviction, nil, evictionKind, mymirrorpod.Namespace, mymirrorpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "",
		},

		// Mirror pods bound to another node
		{
			name:       "forbid creating a mirror pod bound to another",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(othermirrorpod, nil, podKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid update of mirror pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(othermirrorpod, othermirrorpod, podKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of mirror pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "", admission.Delete, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid create of mirror pod status bound to another",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(othermirrorpod, nil, podKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "status", admission.Create, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid update of mirror pod status bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(othermirrorpod, othermirrorpod, podKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "status", admission.Update, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid delete of mirror pod status bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "status", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of eviction for mirror pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(othermirrorpodEviction, nil, evictionKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid update of eviction for mirror pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(othermirrorpodEviction, nil, evictionKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "eviction", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of eviction for mirror pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(othermirrorpodEviction, nil, evictionKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "eviction", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of unnamed eviction for mirror pod to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unnamedEviction, nil, evictionKind, othermirrorpod.Namespace, othermirrorpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},

		// Mirror pods not bound to any node
		{
			name:       "forbid creating a mirror pod unbound",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(unboundmirrorpod, nil, podKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid update of mirror pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundmirrorpod, unboundmirrorpod, podKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of mirror pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "", admission.Delete, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid create of mirror pod status unbound",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(unboundmirrorpod, nil, podKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "status", admission.Create, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid update of mirror pod status unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundmirrorpod, unboundmirrorpod, podKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "status", admission.Update, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid delete of mirror pod status unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "status", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of eviction for mirror pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundmirrorpodEviction, nil, evictionKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid update of eviction for mirror pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundmirrorpodEviction, nil, evictionKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "eviction", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of eviction for mirror pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundmirrorpodEviction, nil, evictionKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "eviction", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of unnamed eviction for mirror pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unnamedEviction, nil, evictionKind, unboundmirrorpod.Namespace, unboundmirrorpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},

		// Normal pods bound to us
		{
			name:       "forbid creating a normal pod bound to self",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mypod, nil, podKind, mypod.Namespace, mypod.Name, podResource, "", admission.Create, mynode),
			err:        "can only create mirror pods",
		},
		{
			name:       "forbid update of normal pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mypod, mypod, podKind, mypod.Namespace, mypod.Name, podResource, "", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "allow delete of normal pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, mypod.Namespace, mypod.Name, podResource, "", admission.Delete, mynode),
			err:        "",
		},
		{
			name:       "forbid create of normal pod status bound to self",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mypod, nil, podKind, mypod.Namespace, mypod.Name, podResource, "status", admission.Create, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "allow update of normal pod status bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mypod, mypod, podKind, mypod.Namespace, mypod.Name, podResource, "status", admission.Update, mynode),
			err:        "",
		},
		{
			name:       "forbid delete of normal pod status bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, mypod.Namespace, mypod.Name, podResource, "status", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid update of eviction for normal pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mypodEviction, nil, evictionKind, mypod.Namespace, mypod.Name, podResource, "eviction", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of eviction for normal pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mypodEviction, nil, evictionKind, mypod.Namespace, mypod.Name, podResource, "eviction", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "allow create of unnamed eviction for normal pod bound to self",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unnamedEviction, nil, evictionKind, mypod.Namespace, mypod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "",
		},

		// Normal pods bound to another
		{
			name:       "forbid creating a normal pod bound to another",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(otherpod, nil, podKind, otherpod.Namespace, otherpod.Name, podResource, "", admission.Create, mynode),
			err:        "can only create mirror pods",
		},
		{
			name:       "forbid update of normal pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(otherpod, otherpod, podKind, otherpod.Namespace, otherpod.Name, podResource, "", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of normal pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, otherpod.Namespace, otherpod.Name, podResource, "", admission.Delete, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid create of normal pod status bound to another",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(otherpod, nil, podKind, otherpod.Namespace, otherpod.Name, podResource, "status", admission.Create, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid update of normal pod status bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(otherpod, otherpod, podKind, otherpod.Namespace, otherpod.Name, podResource, "status", admission.Update, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid delete of normal pod status bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, otherpod.Namespace, otherpod.Name, podResource, "status", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of eviction for normal pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(otherpodEviction, nil, evictionKind, otherpodEviction.Namespace, otherpodEviction.Name, podResource, "eviction", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid update of eviction for normal pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(otherpodEviction, nil, evictionKind, otherpodEviction.Namespace, otherpodEviction.Name, podResource, "eviction", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of eviction for normal pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(otherpodEviction, nil, evictionKind, otherpodEviction.Namespace, otherpodEviction.Name, podResource, "eviction", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of unnamed eviction for normal pod bound to another",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unnamedEviction, nil, evictionKind, otherpod.Namespace, otherpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},

		// Normal pods not bound to any node
		{
			name:       "forbid creating a normal pod unbound",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(unboundpod, nil, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "", admission.Create, mynode),
			err:        "can only create mirror pods",
		},
		{
			name:       "forbid update of normal pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundpod, unboundpod, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of normal pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "", admission.Delete, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid create of normal pod status unbound",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(unboundpod, nil, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "status", admission.Create, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid update of normal pod status unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundpod, unboundpod, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "status", admission.Update, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid delete of normal pod status unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "status", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of eviction for normal pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundpodEviction, nil, evictionKind, unboundpod.Namespace, unboundpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},
		{
			name:       "forbid update of eviction for normal pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundpodEviction, nil, evictionKind, unboundpod.Namespace, unboundpod.Name, podResource, "eviction", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of eviction for normal pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundpodEviction, nil, evictionKind, unboundpod.Namespace, unboundpod.Name, podResource, "eviction", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of unnamed eviction for normal unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unnamedEviction, nil, evictionKind, unboundpod.Namespace, unboundpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "spec.nodeName set to itself",
		},

		// Missing pod
		{
			name:       "forbid delete of unknown pod",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "", admission.Delete, mynode),
			err:        "not found",
		},
		{
			name:       "forbid create of eviction for unknown pod",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mypodEviction, nil, evictionKind, mypod.Namespace, mypod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "not found",
		},
		{
			name:       "forbid update of eviction for unknown pod",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mypodEviction, nil, evictionKind, mypod.Namespace, mypod.Name, podResource, "eviction", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of eviction for unknown pod",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mypodEviction, nil, evictionKind, mypod.Namespace, mypod.Name, podResource, "eviction", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of unnamed eviction for unknown pod",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(unnamedEviction, nil, evictionKind, mypod.Namespace, mypod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "not found",
		},

		// Eviction for unnamed pod
		{
			name:       "allow create of eviction for unnamed pod",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mypodEviction, nil, evictionKind, unnamedpod.Namespace, unnamedpod.Name, podResource, "eviction", admission.Create, mynode),
			// use the submitted eviction resource name as the pod name
			err: "",
		},
		{
			name:       "forbid update of eviction for unnamed pod",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mypodEviction, nil, evictionKind, unnamedpod.Namespace, unnamedpod.Name, podResource, "eviction", admission.Update, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid delete of eviction for unnamed pod",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mypodEviction, nil, evictionKind, unnamedpod.Namespace, unnamedpod.Name, podResource, "eviction", admission.Delete, mynode),
			err:        "forbidden: unexpected operation",
		},
		{
			name:       "forbid create of unnamed eviction for unnamed pod",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unnamedEviction, nil, evictionKind, unnamedpod.Namespace, unnamedpod.Name, podResource, "eviction", admission.Create, mynode),
			err:        "could not determine pod from request data",
		},

		// Resource pods
		{
			name:       "forbid create of pod referencing service account",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(sapod, nil, podKind, sapod.Namespace, sapod.Name, podResource, "", admission.Create, mynode),
			err:        "reference a service account",
		},
		{
			name:       "forbid create of pod referencing secret",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(secretpod, nil, podKind, secretpod.Namespace, secretpod.Name, podResource, "", admission.Create, mynode),
			err:        "reference secrets",
		},
		{
			name:       "forbid create of pod referencing configmap",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(configmappod, nil, podKind, configmappod.Namespace, configmappod.Name, podResource, "", admission.Create, mynode),
			err:        "reference configmaps",
		},
		{
			name:       "forbid create of pod referencing persistentvolumeclaim",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(pvcpod, nil, podKind, pvcpod.Namespace, pvcpod.Name, podResource, "", admission.Create, mynode),
			err:        "reference persistentvolumeclaims",
		},

		// My node object
		{
			name:       "allow create of my node",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mynodeObj, nil, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Create, mynode),
			err:        "",
		},
		{
			name:       "allow create of my node pulling name from object",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mynodeObj, nil, nodeKind, mynodeObj.Namespace, "", nodeResource, "", admission.Create, mynode),
			err:        "",
		},
		{
			name:       "allow create of my node with taints",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mynodeObjTaintA, nil, nodeKind, mynodeObj.Namespace, "", nodeResource, "", admission.Create, mynode),
			err:        "",
		},
		{
			name:       "allow update of my node",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObj, mynodeObj, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "",
		},
		{
			name:       "allow delete of my node",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Delete, mynode),
			err:        "",
		},
		{
			name:       "allow update of my node status",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObj, mynodeObj, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "status", admission.Update, mynode),
			err:        "",
		},
		{
			name:       "forbid create of my node with non-nil configSource",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(mynodeObjConfigA, nil, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Create, mynode),
			err:        "create with non-nil configSource",
		},
		{
			name:       "forbid update of my node: nil configSource to new non-nil configSource",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObjConfigA, mynodeObj, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "update configSource to a new non-nil configSource",
		},
		{
			name:       "forbid update of my node: non-nil configSource to new non-nil configSource",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObjConfigB, mynodeObjConfigA, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "update configSource to a new non-nil configSource",
		},
		{
			name:       "allow update of my node: non-nil configSource unchanged",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObjConfigA, mynodeObjConfigA, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "",
		},
		{
			name:       "allow update of my node: non-nil configSource to nil configSource",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObj, mynodeObjConfigA, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "",
		},
		{
			name:       "allow update of my node: no change to taints",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObjTaintA, mynodeObjTaintA, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "",
		},
		{
			name:       "forbid update of my node: add taints",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObjTaintA, mynodeObj, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "cannot modify taints",
		},
		{
			name:       "forbid update of my node: remove taints",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObj, mynodeObjTaintA, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "cannot modify taints",
		},
		{
			name:       "forbid update of my node: change taints",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(mynodeObjTaintA, mynodeObjTaintB, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "cannot modify taints",
		},
		{
			name:       "allow create to add allowed labels",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedLabels), nil, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Create, mynode),
		},
		{
			name:       "allow update to add allowed labels",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedLabels), mynodeObj, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
		},
		{
			name:       "allow update status to add allowed labels",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedLabels), mynodeObj, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "status", admission.Update, mynode),
		},
		{
			name:       "allow update to add allowed labels and move disallowed labels",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedLabels, disallowedLabels), nil, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Create, mynode),
			expected:   annotate(label(mynodeObj, allowedLabels), disallowedAdditionAnnotation),
		},
		{
			name:       "allow update status to add allowed labels and move disallowed labels",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedLabels, disallowedLabels), nil, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "status", admission.Create, mynode),
			expected:   annotate(label(mynodeObj, allowedLabels), disallowedAdditionAnnotation),
		},
		{
			name:       "allow update to add allowed labels and move disallowed labels",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedLabels, disallowedLabels), mynodeObj, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			expected:   annotate(label(mynodeObj, allowedLabels), disallowedAdditionAnnotation),
		},
		{
			name:       "allow update status to add allowed labels and move disallowed labels",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedLabels, disallowedLabels), mynodeObj, nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "status", admission.Update, mynode),
			expected:   annotate(label(mynodeObj, allowedLabels), disallowedAdditionAnnotation),
		},
		{
			name:       "allow update to change allowed labels and move disallowed changes",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedChangedLabels, disallowedChangedLabels), label(mynodeObj, allowedLabels, disallowedLabels), nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			expected:   annotate(label(mynodeObj, allowedChangedLabels, disallowedLabels), disallowedChangeAnnotation),
		},
		{
			name:       "allow update status to change allowed labels and move disallowed changes",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedChangedLabels, disallowedChangedLabels), label(mynodeObj, allowedLabels, disallowedLabels), nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "status", admission.Update, mynode),
			expected:   annotate(label(mynodeObj, allowedChangedLabels, disallowedLabels), disallowedChangeAnnotation),
		},
		{
			name:       "allow update to change allowed labels and move disallowed changes",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedChangedLabels), label(mynodeObj, allowedLabels, disallowedLabels), nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "", admission.Update, mynode),
			expected:   annotate(label(mynodeObj, allowedChangedLabels, disallowedLabels), disallowedRemovalAnnotation),
		},
		{
			name:       "allow update status to change allowed labels and move disallowed changes",
			attributes: admission.NewAttributesRecord(label(mynodeObj, allowedChangedLabels), label(mynodeObj, allowedLabels, disallowedLabels), nodeKind, mynodeObj.Namespace, mynodeObj.Name, nodeResource, "status", admission.Update, mynode),
			expected:   annotate(label(mynodeObj, allowedChangedLabels, disallowedLabels), disallowedRemovalAnnotation),
		},

		// Other node object
		{
			name:       "forbid create of other node",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(othernodeObj, nil, nodeKind, othernodeObj.Namespace, othernodeObj.Name, nodeResource, "", admission.Create, mynode),
			err:        "cannot modify node",
		},
		{
			name:       "forbid create of other node pulling name from object",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(othernodeObj, nil, nodeKind, othernodeObj.Namespace, "", nodeResource, "", admission.Create, mynode),
			err:        "cannot modify node",
		},
		{
			name:       "forbid update of other node",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(othernodeObj, othernodeObj, nodeKind, othernodeObj.Namespace, othernodeObj.Name, nodeResource, "", admission.Update, mynode),
			err:        "cannot modify node",
		},
		{
			name:       "forbid delete of other node",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, nodeKind, othernodeObj.Namespace, othernodeObj.Name, nodeResource, "", admission.Delete, mynode),
			err:        "cannot modify node",
		},
		{
			name:       "forbid update of other node status",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(othernodeObj, othernodeObj, nodeKind, othernodeObj.Namespace, othernodeObj.Name, nodeResource, "status", admission.Update, mynode),
			err:        "cannot modify node",
		},

		// Service accounts
		{
			name:       "forbid create of unbound token",
			podsGetter: noExistingPods,
			features:   trEnabledFeature,
			attributes: admission.NewAttributesRecord(makeTokenRequest("", ""), nil, tokenrequestKind, "ns", "mysa", svcacctResource, "token", admission.Create, mynode),
			err:        "not bound to a pod",
		},
		{
			name:       "forbid create of token bound to nonexistant pod",
			podsGetter: noExistingPods,
			features:   trEnabledFeature,
			attributes: admission.NewAttributesRecord(makeTokenRequest("nopod", "someuid"), nil, tokenrequestKind, "ns", "mysa", svcacctResource, "token", admission.Create, mynode),
			err:        "not found",
		},
		{
			name:       "forbid create of token bound to pod without uid",
			podsGetter: existingPods,
			features:   trEnabledFeature,
			attributes: admission.NewAttributesRecord(makeTokenRequest(mypod.Name, ""), nil, tokenrequestKind, "ns", "mysa", svcacctResource, "token", admission.Create, mynode),
			err:        "pod binding without a uid",
		},
		{
			name:       "forbid create of token bound to pod scheduled on another node",
			podsGetter: existingPods,
			features:   trEnabledFeature,
			attributes: admission.NewAttributesRecord(makeTokenRequest(otherpod.Name, otherpod.UID), nil, tokenrequestKind, otherpod.Namespace, "mysa", svcacctResource, "token", admission.Create, mynode),
			err:        "pod scheduled on a different node",
		},
		{
			name:       "allow create of token bound to pod scheduled this node",
			podsGetter: existingPods,
			features:   trEnabledFeature,
			attributes: admission.NewAttributesRecord(makeTokenRequest(mypod.Name, mypod.UID), nil, tokenrequestKind, mypod.Namespace, "mysa", svcacctResource, "token", admission.Create, mynode),
		},

		// Unrelated objects
		{
			name:       "allow create of unrelated object",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(&api.ConfigMap{}, nil, configmapKind, "myns", "mycm", configmapResource, "", admission.Create, mynode),
			err:        "",
		},
		{
			name:       "allow update of unrelated object",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(&api.ConfigMap{}, &api.ConfigMap{}, configmapKind, "myns", "mycm", configmapResource, "", admission.Update, mynode),
			err:        "",
		},
		{
			name:       "allow delete of unrelated object",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, configmapKind, "myns", "mycm", configmapResource, "", admission.Delete, mynode),
			err:        "",
		},

		// Unrelated user
		{
			name:       "allow unrelated user creating a normal pod unbound",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(unboundpod, nil, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "", admission.Create, bob),
			err:        "",
		},
		{
			name:       "allow unrelated user update of normal pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundpod, unboundpod, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "", admission.Update, bob),
			err:        "",
		},
		{
			name:       "allow unrelated user delete of normal pod unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "", admission.Delete, bob),
			err:        "",
		},
		{
			name:       "allow unrelated user create of normal pod status unbound",
			podsGetter: noExistingPods,
			attributes: admission.NewAttributesRecord(unboundpod, nil, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "status", admission.Create, bob),
			err:        "",
		},
		{
			name:       "allow unrelated user update of normal pod status unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(unboundpod, unboundpod, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "status", admission.Update, bob),
			err:        "",
		},
		{
			name:       "allow unrelated user delete of normal pod status unbound",
			podsGetter: existingPods,
			attributes: admission.NewAttributesRecord(nil, nil, podKind, unboundpod.Namespace, unboundpod.Name, podResource, "status", admission.Delete, bob),
			err:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// record our starting object
			startingObj := tt.attributes.GetObject()
			if startingObj != nil {
				startingObj = startingObj.DeepCopyObject()
			}
			defer func() {
				// make sure any mutations are expected
				expectedObject := tt.expected
				if expectedObject == nil {
					expectedObject = startingObj
				}
				if !reflect.DeepEqual(expectedObject, tt.attributes.GetObject()) {
					t.Errorf("unexpected result:\n%s", diff.ObjectReflectDiff(expectedObject, tt.attributes.GetObject()))
				}
			}()

			c := NewPlugin(nodeidentifier.NewDefaultNodeIdentifier())
			if tt.features != nil {
				c.features = tt.features
			}
			c.podsGetter = tt.podsGetter
			err := c.Admit(tt.attributes)
			if (err == nil) != (len(tt.err) == 0) {
				t.Errorf("nodePlugin.Admit() error = %v, expected %v", err, tt.err)
				return
			}
			if len(tt.err) > 0 && !strings.Contains(err.Error(), tt.err) {
				t.Errorf("nodePlugin.Admit() error = %v, expected %v", err, tt.err)
			}
		})
	}
}
