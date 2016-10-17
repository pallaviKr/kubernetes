/*
Copyright 2014 The Kubernetes Authors.

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

package pod

import (
	"net/url"
	"reflect"
	"testing"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/errors"
	apitesting "k8s.io/kubernetes/pkg/api/testing"
	"k8s.io/kubernetes/pkg/apimachinery/registered"
	metav1 "k8s.io/kubernetes/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/fields"
	genericapirequest "k8s.io/kubernetes/pkg/genericapiserver/api/request"
	"k8s.io/kubernetes/pkg/kubelet/client"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/types"
)

func TestMatchPod(t *testing.T) {
	testCases := []struct {
		in            *api.Pod
		fieldSelector fields.Selector
		expectMatch   bool
	}{
		{
			in: &api.Pod{
				Spec: api.PodSpec{NodeName: "nodeA"},
			},
			fieldSelector: fields.ParseSelectorOrDie("spec.nodeName=nodeA"),
			expectMatch:   true,
		},
		{
			in: &api.Pod{
				Spec: api.PodSpec{NodeName: "nodeB"},
			},
			fieldSelector: fields.ParseSelectorOrDie("spec.nodeName=nodeA"),
			expectMatch:   false,
		},
		{
			in: &api.Pod{
				Spec: api.PodSpec{RestartPolicy: api.RestartPolicyAlways},
			},
			fieldSelector: fields.ParseSelectorOrDie("spec.restartPolicy=Always"),
			expectMatch:   true,
		},
		{
			in: &api.Pod{
				Spec: api.PodSpec{RestartPolicy: api.RestartPolicyAlways},
			},
			fieldSelector: fields.ParseSelectorOrDie("spec.restartPolicy=Never"),
			expectMatch:   false,
		},
		{
			in: &api.Pod{
				Status: api.PodStatus{Phase: api.PodRunning},
			},
			fieldSelector: fields.ParseSelectorOrDie("status.phase=Running"),
			expectMatch:   true,
		},
		{
			in: &api.Pod{
				Status: api.PodStatus{Phase: api.PodRunning},
			},
			fieldSelector: fields.ParseSelectorOrDie("status.phase=Pending"),
			expectMatch:   false,
		},
	}
	for _, testCase := range testCases {
		m := MatchPod(labels.Everything(), testCase.fieldSelector)
		result, err := m.Matches(testCase.in)
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		}
		if result != testCase.expectMatch {
			t.Errorf("Result %v, Expected %v, Selector: %v, Pod: %v", result, testCase.expectMatch, testCase.fieldSelector.String(), testCase.in)
		}
	}
}

func TestCheckGracefulDelete(t *testing.T) {
	defaultGracePeriod := int64(30)
	tcs := []struct {
		in          *api.Pod
		gracePeriod int64
	}{
		{
			in: &api.Pod{
				Spec:   api.PodSpec{NodeName: "something"},
				Status: api.PodStatus{Phase: api.PodPending},
			},

			gracePeriod: defaultGracePeriod,
		},
		{
			in: &api.Pod{
				Spec:   api.PodSpec{NodeName: "something"},
				Status: api.PodStatus{Phase: api.PodFailed},
			},
			gracePeriod: 0,
		},
		{
			in: &api.Pod{
				Spec:   api.PodSpec{},
				Status: api.PodStatus{Phase: api.PodPending},
			},
			gracePeriod: 0,
		},
		{
			in: &api.Pod{
				Spec:   api.PodSpec{},
				Status: api.PodStatus{Phase: api.PodSucceeded},
			},
			gracePeriod: 0,
		},
		{
			in: &api.Pod{
				Spec:   api.PodSpec{},
				Status: api.PodStatus{},
			},
			gracePeriod: 0,
		},
	}
	for _, tc := range tcs {
		out := &api.DeleteOptions{GracePeriodSeconds: &defaultGracePeriod}
		Strategy.CheckGracefulDelete(genericapirequest.NewContext(), tc.in, out)
		if out.GracePeriodSeconds == nil {
			t.Errorf("out grace period was nil but supposed to be %v", tc.gracePeriod)
		}
		if *(out.GracePeriodSeconds) != tc.gracePeriod {
			t.Errorf("out grace period was %v but was expected to be %v", *out, tc.gracePeriod)
		}
	}
}

type mockPodGetter struct {
	pod *api.Pod
}

func (g mockPodGetter) Get(genericapirequest.Context, string, *metav1.GetOptions) (runtime.Object, error) {
	return g.pod, nil
}

func TestCheckLogLocation(t *testing.T) {
	ctx := genericapirequest.NewDefaultContext()
	tcs := []struct {
		in          *api.Pod
		opts        *api.PodLogOptions
		expectedErr error
	}{
		{
			in: &api.Pod{
				Spec:   api.PodSpec{},
				Status: api.PodStatus{},
			},
			opts:        &api.PodLogOptions{},
			expectedErr: errors.NewBadRequest("a container name must be specified for pod test"),
		},
		{
			in: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{
						{Name: "mycontainer"},
					},
				},
				Status: api.PodStatus{},
			},
			opts:        &api.PodLogOptions{},
			expectedErr: nil,
		},
		{
			in: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{
						{Name: "container1"},
						{Name: "container2"},
					},
				},
				Status: api.PodStatus{},
			},
			opts:        &api.PodLogOptions{},
			expectedErr: errors.NewBadRequest("a container name must be specified for pod test, choose one of: [container1 container2]"),
		},
		{
			in: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{
						{Name: "container1"},
						{Name: "container2"},
					},
					InitContainers: []api.Container{
						{Name: "initcontainer1"},
					},
				},
				Status: api.PodStatus{},
			},
			opts:        &api.PodLogOptions{},
			expectedErr: errors.NewBadRequest("a container name must be specified for pod test, choose one of: [container1 container2] or one of the init containers: [initcontainer1]"),
		},
		{
			in: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{
						{Name: "container1"},
						{Name: "container2"},
					},
				},
				Status: api.PodStatus{},
			},
			opts: &api.PodLogOptions{
				Container: "unknown",
			},
			expectedErr: errors.NewBadRequest("container unknown is not valid for pod test"),
		},
		{
			in: &api.Pod{
				Spec: api.PodSpec{
					Containers: []api.Container{
						{Name: "container1"},
						{Name: "container2"},
					},
				},
				Status: api.PodStatus{},
			},
			opts: &api.PodLogOptions{
				Container: "container2",
			},
			expectedErr: nil,
		},
	}
	for _, tc := range tcs {
		getter := &mockPodGetter{tc.in}
		_, _, err := LogLocation(getter, nil, ctx, "test", tc.opts)
		if !reflect.DeepEqual(err, tc.expectedErr) {
			t.Errorf("expected %v, got %v", tc.expectedErr, err)
		}
	}
}

func TestSelectableFieldLabelConversions(t *testing.T) {
	apitesting.TestSelectableFieldLabelConversionsOfKind(t,
		registered.GroupOrDie(api.GroupName).GroupVersion.String(),
		"Pod",
		PodToSelectableFields(&api.Pod{}),
		nil,
	)
}

type mockConnectionInfoGetter struct {
	info *client.ConnectionInfo
}

func (g mockConnectionInfoGetter) GetConnectionInfo(nodeName types.NodeName) (*client.ConnectionInfo, error) {
	return g.info, nil
}

func TestPortForwardLocation(t *testing.T) {
	ctx := genericapirequest.NewDefaultContext()
	tcs := []struct {
		in          *api.Pod
		info        *client.ConnectionInfo
		opts        *api.PodPortForwardOptions
		expectedErr error
		expectedURL *url.URL
	}{
		{
			in: &api.Pod{
				Spec: api.PodSpec{},
			},
			opts:        &api.PodPortForwardOptions{},
			expectedErr: errors.NewBadRequest("pod test does not have a host assigned"),
		},
		{
			in: &api.Pod{
				Spec: api.PodSpec{
					NodeName: "node1",
				},
			},
			opts:        &api.PodPortForwardOptions{},
			expectedErr: errors.NewBadRequest("at least one port must be specified"),
		},
		{
			in: &api.Pod{
				ObjectMeta: api.ObjectMeta{
					Namespace: "ns",
					Name:      "pod1",
				},
				Spec: api.PodSpec{
					NodeName: "node1",
				},
			},
			info:        &client.ConnectionInfo{},
			opts:        &api.PodPortForwardOptions{Ports: []int32{80}},
			expectedURL: &url.URL{Host: ":", Path: "/portForward/ns/pod1", RawQuery: "port=80"},
		},
	}
	for _, tc := range tcs {
		getter := &mockPodGetter{tc.in}
		connectionGetter := &mockConnectionInfoGetter{tc.info}
		loc, _, err := PortForwardLocation(getter, connectionGetter, ctx, "test", tc.opts)
		if !reflect.DeepEqual(err, tc.expectedErr) {
			t.Errorf("expected %v, got %v", tc.expectedErr, err)
		}
		if !reflect.DeepEqual(loc, tc.expectedURL) {
			t.Errorf("expected %v, got %v", tc.expectedURL, loc)
		}
	}
}
