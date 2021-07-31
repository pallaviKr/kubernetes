/*
Copyright 2015 The Kubernetes Authors.

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

package storage

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	machineryutilnet "k8s.io/apimachinery/pkg/util/net"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistrytest "k8s.io/apiserver/pkg/registry/generic/testing"
	"k8s.io/apiserver/pkg/registry/rest"
	etcd3testing "k8s.io/apiserver/pkg/storage/etcd3/testing"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	svctest "k8s.io/kubernetes/pkg/api/service/testing"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/registry/core/service/ipallocator"
	"k8s.io/kubernetes/pkg/registry/core/service/portallocator"
	"k8s.io/kubernetes/pkg/registry/registrytest"
	utilnet "k8s.io/utils/net"
)

func makeIPAllocator(cidr *net.IPNet) ipallocator.Interface {
	al, err := ipallocator.NewInMemory(cidr)
	if err != nil {
		panic(fmt.Sprintf("error creating IP allocator: %v", err))
	}
	return al
}

func makePortAllocator(ports machineryutilnet.PortRange) portallocator.Interface {
	al, err := portallocator.NewInMemory(ports)
	if err != nil {
		panic(fmt.Sprintf("error creating port allocator: %v", err))
	}
	return al
}

type fakeEndpoints struct{}

func (fakeEndpoints) Delete(_ context.Context, _ string, _ rest.ValidateObjectFunc, _ *metav1.DeleteOptions) (runtime.Object, bool, error) {
	return nil, false, nil
}

func (fakeEndpoints) Get(_ context.Context, _ string, _ *metav1.GetOptions) (runtime.Object, error) {
	return nil, nil
}

func ipIsAllocated(t *testing.T, alloc ipallocator.Interface, ipstr string) bool {
	t.Helper()
	ip := net.ParseIP(ipstr)
	if ip == nil {
		t.Errorf("error parsing IP %q", ipstr)
		return false
	}
	return alloc.Has(ip)
}

func portIsAllocated(t *testing.T, alloc portallocator.Interface, port int32) bool {
	t.Helper()
	if port == 0 {
		t.Errorf("port is 0")
		return false
	}
	return alloc.Has(int(port))
}

func newStorage(t *testing.T, ipFamilies []api.IPFamily) (*GenericREST, *StatusREST, *etcd3testing.EtcdTestServer) {
	etcdStorage, server := registrytest.NewEtcdStorage(t, "")
	restOptions := generic.RESTOptions{
		StorageConfig:           etcdStorage,
		Decorator:               generic.UndecoratedStorage,
		DeleteCollectionWorkers: 1,
		ResourcePrefix:          "services",
	}

	ipAllocs := map[api.IPFamily]ipallocator.Interface{}
	for _, fam := range ipFamilies {
		switch fam {
		case api.IPv4Protocol:
			_, cidr, _ := net.ParseCIDR("10.0.0.0/16")
			ipAllocs[fam] = makeIPAllocator(cidr)
		case api.IPv6Protocol:
			_, cidr, _ := net.ParseCIDR("2000::/108")
			ipAllocs[fam] = makeIPAllocator(cidr)
		default:
			t.Fatalf("Unknown IPFamily: %v", fam)
		}
	}

	portAlloc := makePortAllocator(*(machineryutilnet.ParsePortRangeOrDie("30000-32767")))

	serviceStorage, statusStorage, err := NewGenericREST(restOptions, ipFamilies[0], ipAllocs, portAlloc, fakeEndpoints{})
	if err != nil {
		t.Fatalf("unexpected error from REST storage: %v", err)
	}
	return serviceStorage, statusStorage, server
}

// This is used in generic registry tests.
func validService() *api.Service {
	return svctest.MakeService("foo",
		svctest.SetClusterIPs(api.ClusterIPNone),
		svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
		svctest.SetIPFamilies(api.IPv4Protocol))
}

func TestGenericCreate(t *testing.T) {
	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)
	svc := validService()
	svc.ObjectMeta = metav1.ObjectMeta{} // because genericregistrytest
	test.TestCreate(
		// valid
		svc,
		// invalid
		&api.Service{
			Spec: api.ServiceSpec{},
		},
	)
}

func TestGenericUpdate(t *testing.T) {
	clusterInternalTrafficPolicy := api.ServiceInternalTrafficPolicyCluster

	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).AllowCreateOnUpdate()
	test.TestUpdate(
		// valid
		validService(),
		// updateFunc
		func(obj runtime.Object) runtime.Object {
			object := obj.(*api.Service)
			object.Spec = api.ServiceSpec{
				Selector:        map[string]string{"bar": "baz2"},
				ClusterIP:       api.ClusterIPNone,
				ClusterIPs:      []string{api.ClusterIPNone},
				SessionAffinity: api.ServiceAffinityNone,
				Type:            api.ServiceTypeClusterIP,
				Ports: []api.ServicePort{{
					Port:       6502,
					Protocol:   api.ProtocolTCP,
					TargetPort: intstr.FromInt(6502),
				}},
				InternalTrafficPolicy: &clusterInternalTrafficPolicy,
			}
			return object
		},
	)
}

func TestGenericDelete(t *testing.T) {
	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).AllowCreateOnUpdate().ReturnDeletedObject()
	test.TestDelete(validService())
}

func TestGenericGet(t *testing.T) {
	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).AllowCreateOnUpdate()
	test.TestGet(validService())
}

func TestGenericList(t *testing.T) {
	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).AllowCreateOnUpdate()
	test.TestList(validService())
}

func TestGenericWatch(t *testing.T) {
	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)
	test.TestWatch(
		validService(),
		// matching labels
		[]labels.Set{},
		// not matching labels
		[]labels.Set{
			{"foo": "bar"},
		},
		// matching fields
		[]fields.Set{
			{"metadata.name": "foo"},
		},
		// not matching fields
		[]fields.Set{
			{"metadata.name": "bar"},
		},
	)
}

func TestGenericShortNames(t *testing.T) {
	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	expected := []string{"svc"}
	registrytest.AssertShortNames(t, storage, expected)
}

func TestGenericCategories(t *testing.T) {
	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	expected := []string{"all"}
	registrytest.AssertCategories(t, storage, expected)
}

func TestServiceDefaultOnRead(t *testing.T) {
	// Helper makes a mostly-valid ServiceList.  Test-cases can tweak it as needed.
	makeServiceList := func(tweaks ...svctest.Tweak) *api.ServiceList {
		svc := svctest.MakeService("foo", tweaks...)
		list := &api.ServiceList{
			Items: []api.Service{*svc},
		}
		return list
	}

	testCases := []struct {
		name   string
		input  runtime.Object
		expect runtime.Object
	}{{
		name:   "no change v4",
		input:  svctest.MakeService("foo", svctest.SetClusterIPs("10.0.0.1")),
		expect: svctest.MakeService("foo", svctest.SetClusterIPs("10.0.0.1")),
	}, {
		name:   "missing clusterIPs v4",
		input:  svctest.MakeService("foo", svctest.SetClusterIP("10.0.0.1")),
		expect: svctest.MakeService("foo", svctest.SetClusterIPs("10.0.0.1")),
	}, {
		name:   "no change v6",
		input:  svctest.MakeService("foo", svctest.SetClusterIPs("2000::1")),
		expect: svctest.MakeService("foo", svctest.SetClusterIPs("2000::1")),
	}, {
		name:   "missing clusterIPs v6",
		input:  svctest.MakeService("foo", svctest.SetClusterIP("2000::1")),
		expect: svctest.MakeService("foo", svctest.SetClusterIPs("2000::1")),
	}, {
		name:   "list, no change v4",
		input:  makeServiceList(svctest.SetClusterIPs("10.0.0.1")),
		expect: makeServiceList(svctest.SetClusterIPs("10.0.0.1")),
	}, {
		name:   "list, missing clusterIPs v4",
		input:  makeServiceList(svctest.SetClusterIP("10.0.0.1")),
		expect: makeServiceList(svctest.SetClusterIPs("10.0.0.1")),
	}, {
		name:  "not Service or ServiceList",
		input: &api.Pod{},
	}}

	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			tmp := tc.input.DeepCopyObject()
			storage.defaultOnRead(tmp)

			svc, ok := tmp.(*api.Service)
			if !ok {
				list, ok := tmp.(*api.ServiceList)
				if !ok {
					return
				}
				svc = &list.Items[0]
			}

			exp, ok := tc.expect.(*api.Service)
			if !ok {
				list, ok := tc.expect.(*api.ServiceList)
				if !ok {
					return
				}
				exp = &list.Items[0]
			}

			// Verify fields we know are affected
			if svc.Spec.ClusterIP != exp.Spec.ClusterIP {
				t.Errorf("clusterIP: expected %v, got %v", exp.Spec.ClusterIP, svc.Spec.ClusterIP)
			}
			if !reflect.DeepEqual(svc.Spec.ClusterIPs, exp.Spec.ClusterIPs) {
				t.Errorf("clusterIPs: expected %v, got %v", exp.Spec.ClusterIPs, svc.Spec.ClusterIPs)
			}
		})
	}
}

func fmtIPFamilyPolicy(pol *api.IPFamilyPolicyType) string {
	if pol == nil {
		return "<nil>"
	}
	return string(*pol)
}

func fmtIPFamilies(fams []api.IPFamily) string {
	if fams == nil {
		return "[]"
	}
	return fmt.Sprintf("%v", fams)
}

func familyOf(ip string) api.IPFamily {
	if utilnet.IsIPv4String(ip) {
		return api.IPv4Protocol
	}
	if utilnet.IsIPv6String(ip) {
		return api.IPv6Protocol
	}
	return api.IPFamily("unknown")
}

// Prove that create ignores IP and IPFamily stuff when type is ExternalName.
func TestCreateIgnoresIPsForExternalName(t *testing.T) {
	type testCase struct {
		name        string
		svc         *api.Service
		expectError bool
	}
	// These cases were chosen from the full gamut to ensure all "interesting"
	// cases are covered.
	testCases := []struct {
		name            string
		clusterFamilies []api.IPFamily
		enableDualStack bool
		cases           []testCase
	}{{
		name:            "singlestack:v4_gate:off",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		enableDualStack: false,
		cases: []testCase{{
			name: "Policy:unset_Families:unset",
			svc:  svctest.MakeService("foo"),
		}, {
			name: "Policy:SingleStack_Families:v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
				svctest.SetIPFamilies(api.IPv4Protocol)),
		}, {
			name: "Policy:PreferDualStack_Families:v4v6",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
				svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
		}, {
			name: "Policy:RequireDualStack_Families:v6v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
				svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
		}},
	}, {
		name:            "singlestack:v6_gate:on",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol},
		enableDualStack: true,
		cases: []testCase{{
			name: "Policy:unset_Families:unset",
			svc:  svctest.MakeService("foo"),
		}, {
			name: "Policy:SingleStack_Families:v6",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
				svctest.SetIPFamilies(api.IPv6Protocol)),
			expectError: true,
		}, {
			name: "Policy:PreferDualStack_Families:v4v6",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
				svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
			expectError: true,
		}, {
			name: "Policy:RequireDualStack_Families:v6v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
				svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
			expectError: true,
		}},
	}, {
		name:            "dualstack:v4v6_gate:off",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
		enableDualStack: false,
		cases: []testCase{{
			name: "Policy:unset_Families:unset",
			svc:  svctest.MakeService("foo"),
		}, {
			name: "Policy:SingleStack_Families:v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
				svctest.SetIPFamilies(api.IPv4Protocol)),
		}, {
			name: "Policy:PreferDualStack_Families:v4v6",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
				svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
		}, {
			name: "Policy:RequireDualStack_Families:v6v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
				svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
		}},
	}, {
		name:            "dualstack:v6v4_gate:on",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
		enableDualStack: true,
		cases: []testCase{{
			name: "Policy:unset_Families:unset",
			svc:  svctest.MakeService("foo"),
		}, {
			name: "Policy:SingleStack_Families:v6",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
				svctest.SetIPFamilies(api.IPv6Protocol)),
			expectError: true,
		}, {
			name: "Policy:PreferDualStack_Families:v6v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
				svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
			expectError: true,
		}, {
			name: "Policy:RequireDualStack_Families:v4v6",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
				svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
			expectError: true,
		}},
	}}

	for _, otc := range testCases {
		t.Run(otc.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, otc.enableDualStack)()

			storage, _, server := newStorage(t, otc.clusterFamilies)
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			for _, itc := range otc.cases {
				t.Run(itc.name, func(t *testing.T) {
					// This test is ONLY ExternalName services.
					itc.svc.Spec.Type = api.ServiceTypeExternalName
					itc.svc.Spec.ExternalName = "example.com"

					ctx := genericapirequest.NewDefaultContext()
					createdObj, err := storage.Create(ctx, itc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
					if itc.expectError && err != nil {
						return
					}
					if err != nil {
						t.Fatalf("unexpected error creating service: %v", err)
					}
					defer storage.Delete(ctx, itc.svc.Name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
					if itc.expectError && err == nil {
						t.Fatalf("unexpected success creating service")
					}
					createdSvc := createdObj.(*api.Service)

					if want, got := fmtIPFamilyPolicy(nil), fmtIPFamilyPolicy(createdSvc.Spec.IPFamilyPolicy); want != got {
						t.Errorf("wrong IPFamilyPolicy: want %s, got %s", want, got)
					}
					if want, got := fmtIPFamilies(nil), fmtIPFamilies(createdSvc.Spec.IPFamilies); want != got {
						t.Errorf("wrong IPFamilies: want %s, got %s", want, got)
					}
					if len(createdSvc.Spec.ClusterIP) != 0 {
						t.Errorf("expected no clusterIP, got %q", createdSvc.Spec.ClusterIP)
					}
					if len(createdSvc.Spec.ClusterIPs) != 0 {
						t.Errorf("expected no clusterIPs, got %q", createdSvc.Spec.ClusterIPs)
					}
				})
			}
		})
	}
}

// Prove that create ignores IPFamily stuff when dual-stack is disabled.
func TestCreateIgnoresIPFamilyWithoutDualStack(t *testing.T) {
	// These cases were chosen from the full gamut to ensure all "interesting"
	// cases are covered.
	testCases := []struct {
		name string
		svc  *api.Service
	}{
		//----------------------------------------
		// ClusterIP:unset
		//----------------------------------------
		{
			name: "ClusterIP:unset_Policy:unset_Families:unset",
			svc:  svctest.MakeService("foo"),
		}, {
			name: "ClusterIP:unset_Policy:unset_Families:v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilies(api.IPv4Protocol)),
		}, {
			name: "ClusterIP:unset_Policy:unset_Families:v6v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
		}, {
			name: "ClusterIP:unset_Policy:SingleStack_Families:unset",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
		}, {
			name: "ClusterIP:unset_Policy:SingleStack_Families:v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
				svctest.SetIPFamilies(api.IPv4Protocol)),
		}, {
			name: "ClusterIP:unset_Policy:SingleStack_Families:v6v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
				svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
		}, {
			name: "ClusterIP:unset_Policy:PreferDualStack_Families:unset",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
		}, {
			name: "ClusterIP:unset_Policy:PreferDualStack_Families:v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
				svctest.SetIPFamilies(api.IPv4Protocol)),
		}, {
			name: "ClusterIP:unset_Policy:PreferDualStack_Families:v6v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
				svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
		}, {
			name: "ClusterIP:unset_Policy:RequireDualStack_Families:unset",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
		}, {
			name: "ClusterIP:unset_Policy:RequireDualStack_Families:v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
				svctest.SetIPFamilies(api.IPv4Protocol)),
		}, {
			name: "ClusterIP:unset_Policy:RequireDualStack_Families:v6v4",
			svc: svctest.MakeService("foo",
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
				svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
		},
		//----------------------------------------
		// ClusterIPs:v4v6
		//----------------------------------------
		{
			name: "ClusterIPs:v4v6_Policy:unset_Families:unset",
			svc: svctest.MakeService("foo",
				svctest.SetClusterIPs("10.0.0.1", "2000::1")),
		}, {
			name: "ClusterIPs:v4v6_Policy:unset_Families:v4",
			svc: svctest.MakeService("foo",
				svctest.SetClusterIPs("10.0.0.1", "2000::1"),
				svctest.SetIPFamilies(api.IPv4Protocol)),
		}, {
			name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:unset",
			svc: svctest.MakeService("foo",
				svctest.SetClusterIPs("10.0.0.1", "2000::1"),
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
		},
		//----------------------------------------
		// Headless
		//----------------------------------------
		{
			name: "Headless_Policy:unset_Families:unset",
			svc: svctest.MakeService("foo",
				svctest.SetHeadless),
		}, {
			name: "Headless_Policy:unset_Families:v4",
			svc: svctest.MakeService("foo",
				svctest.SetHeadless,
				svctest.SetIPFamilies(api.IPv4Protocol)),
		}, {
			name: "Headless_Policy:RequireDualStack_Families:unset",
			svc: svctest.MakeService("foo",
				svctest.SetHeadless,
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
		},
		//----------------------------------------
		// HeadlessSelectorless
		//----------------------------------------
		{
			name: "HeadlessSelectorless_Policy:unset_Families:unset",
			svc: svctest.MakeService("foo",
				svctest.SetHeadless,
				svctest.SetSelector(nil)),
		}, {
			name: "HeadlessSelectorless_Policy:unset_Families:v4",
			svc: svctest.MakeService("foo",
				svctest.SetHeadless,
				svctest.SetSelector(nil),
				svctest.SetIPFamilies(api.IPv4Protocol)),
		}, {
			name: "HeadlessSelectorless_Policy:RequireDualStack_Families:unset",
			svc: svctest.MakeService("foo",
				svctest.SetHeadless,
				svctest.SetSelector(nil),
				svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
		},
	}

	// This test is ONLY with the gate off.
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, false)()

	// Do this in the outer scope for performance.
	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := genericapirequest.NewDefaultContext()
			createdObj, err := storage.Create(ctx, tc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}
			defer storage.Delete(ctx, tc.svc.Name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
			createdSvc := createdObj.(*api.Service)

			// The gate is off - these should always be empty.
			if want, got := fmtIPFamilyPolicy(nil), fmtIPFamilyPolicy(createdSvc.Spec.IPFamilyPolicy); want != got {
				t.Errorf("wrong IPFamilyPolicy: want %s, got %s", want, got)
			}
			if want, got := fmtIPFamilies(nil), fmtIPFamilies(createdSvc.Spec.IPFamilies); want != got {
				t.Errorf("wrong IPFamilies: want %s, got %s", want, got)
			}
		})
	}
}

// Prove that create initializes clusterIPs from clusterIP.  This simplifies
// later tests to not need to re-prove this.
func TestCreateInitClusterIPsFromClusterIP(t *testing.T) {
	testCases := []struct {
		name            string
		clusterFamilies []api.IPFamily
		svc             *api.Service
	}{{
		name:            "singlestack:v4_clusterip:unset",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "singlestack:v4_clusterip:set",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		svc: svctest.MakeService("foo",
			svctest.SetClusterIP("10.0.0.1")),
	}, {
		name:            "singlestack:v6_clusterip:unset",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol},
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "singlestack:v6_clusterip:set",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol},
		svc: svctest.MakeService("foo",
			svctest.SetClusterIP("2000::1")),
	}, {
		name:            "dualstack:v4v6_clusterip:unset",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "dualstack:v4v6_clusterip:set",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
		svc: svctest.MakeService("foo",
			svctest.SetClusterIP("10.0.0.1")),
	}, {
		name:            "dualstack:v6v4_clusterip:unset",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "dualstack:v6v4_clusterip:set",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
		svc: svctest.MakeService("foo",
			svctest.SetClusterIP("2000::1")),
	}}

	// This test is ONLY with the gate enabled.
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			storage, _, server := newStorage(t, tc.clusterFamilies)
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			ctx := genericapirequest.NewDefaultContext()
			createdObj, err := storage.Create(ctx, tc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}
			createdSvc := createdObj.(*api.Service)

			if createdSvc.Spec.ClusterIP == "" {
				t.Errorf("expected ClusterIP to be set")

			}
			if tc.svc.Spec.ClusterIP != "" {
				if want, got := tc.svc.Spec.ClusterIP, createdSvc.Spec.ClusterIP; want != got {
					t.Errorf("wrong ClusterIP: want %s, got %s", want, got)
				}
			}
			if len(createdSvc.Spec.ClusterIPs) == 0 {
				t.Errorf("expected ClusterIPs to be set")
			}
			if want, got := createdSvc.Spec.ClusterIP, createdSvc.Spec.ClusterIPs[0]; want != got {
				t.Errorf("wrong ClusterIPs[0]: want %s, got %s", want, got)
			}
		})
	}
}

// Prove that create initializes IPFamily fields correctly.
func TestCreateInitIPFields(t *testing.T) {
	type testCase struct {
		name           string
		svc            *api.Service
		expectError    bool
		expectPolicy   api.IPFamilyPolicyType
		expectFamilies []api.IPFamily
		expectHeadless bool
	}
	// These cases were chosen from the full gamut to ensure all "interesting"
	// cases are covered.
	testCases := []struct {
		name            string
		clusterFamilies []api.IPFamily
		cases           []testCase
	}{
		{
			name:            "singlestack:v4",
			clusterFamilies: []api.IPFamily{api.IPv4Protocol},
			cases: []testCase{
				//----------------------------------------
				// singlestack:v4 ClusterIPs:unset
				//----------------------------------------
				{
					name:           "ClusterIPs:unset_Policy:unset_Families:unset",
					svc:            svctest.MakeService("foo"),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v4 ClusterIPs:v4
				//----------------------------------------
				{
					name: "ClusterIPs:v4_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1")),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v4 ClusterIPs:v4v6
				//----------------------------------------
				{
					name: "ClusterIPs:v4v6_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1")),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v4 ClusterIPs:v6v4
				//----------------------------------------
				{
					name: "ClusterIPs:v6v4_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1")),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v4 Headless
				//----------------------------------------
				{
					name: "Headless_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v4 HeadlessSelectorless
				//----------------------------------------
				{
					name: "HeadlessSelectorless_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				},
			},
		}, {
			name:            "singlestack:v6",
			clusterFamilies: []api.IPFamily{api.IPv6Protocol},
			cases: []testCase{
				//----------------------------------------
				// singlestack:v6 ClusterIPs:unset
				//----------------------------------------
				{
					name:           "ClusterIPs:unset_Policy:unset_Families:unset",
					svc:            svctest.MakeService("foo"),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v6 ClusterIPs:v6
				//----------------------------------------
				{
					name: "ClusterIPs:v6_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1")),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v6 ClusterIPs:v4v6
				//----------------------------------------
				{
					name: "ClusterIPs:v4v6_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1")),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v6 ClusterIPs:v6v4
				//----------------------------------------
				{
					name: "ClusterIPs:v6v4_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1")),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v6 Headless
				//----------------------------------------
				{
					name: "Headless_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// singlestack:v6 HeadlessSelectorless
				//----------------------------------------
				{
					name: "HeadlessSelectorless_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				},
			},
		}, {
			name:            "dualstack:v4v6",
			clusterFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
			cases: []testCase{
				//----------------------------------------
				// dualstack:v4v6 ClusterIPs:unset
				//----------------------------------------
				{
					name:           "ClusterIPs:unset_Policy:unset_Families:unset",
					svc:            svctest.MakeService("foo"),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				},
				//----------------------------------------
				// dualstack:v4v6 ClusterIPs:v4
				//----------------------------------------
				{
					name: "ClusterIPs:v4_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1")),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// dualstack:v4v6 ClusterIPs:v6
				//----------------------------------------
				{
					name: "ClusterIPs:v6_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1")),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				},
				//----------------------------------------
				// dualstack:v4v6 ClusterIPs:v4v6
				//----------------------------------------
				{
					name: "ClusterIPs:v4v6_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1")),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// dualstack:v4v6 ClusterIPs:v6v4
				//----------------------------------------
				{
					name: "ClusterIPs:v6v4_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1")),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				},
				//----------------------------------------
				// dualstack:v4v6 Headless
				//----------------------------------------
				{
					name: "Headless_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				},
				//----------------------------------------
				// dualstack:v4v6 HeadlessSelectorless
				//----------------------------------------
				{
					name: "HeadlessSelectorless_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				},
			},
		}, {
			name:            "dualstack:v6v4",
			clusterFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
			cases: []testCase{
				//----------------------------------------
				// dualstack:v6v4 ClusterIPs:unset
				//----------------------------------------
				{
					name:           "ClusterIPs:unset_Policy:unset_Families:unset",
					svc:            svctest.MakeService("foo"),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:unset_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				},
				//----------------------------------------
				// dualstack:v6v4 ClusterIPs:v4
				//----------------------------------------
				{
					name: "ClusterIPs:v4_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1")),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// dualstack:v6v4 ClusterIPs:v6
				//----------------------------------------
				{
					name: "ClusterIPs:v6_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1")),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				},
				//----------------------------------------
				// dualstack:v6v4 ClusterIPs:v4v6
				//----------------------------------------
				{
					name: "ClusterIPs:v4v6_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1")),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
				}, {
					name: "ClusterIPs:v4v6_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("10.0.0.1", "2000::1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				},
				//----------------------------------------
				// dualstack:v6v4 ClusterIPs:v6v4
				//----------------------------------------
				{
					name: "ClusterIPs:v6v4_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1")),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "ClusterIPs:v6v4_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetClusterIPs("2000::1", "10.0.0.1"),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
				},
				//----------------------------------------
				// dualstack:v6v4 Headless
				//----------------------------------------
				{
					name: "Headless_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "Headless_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				},
				//----------------------------------------
				// dualstack:v6v4 HeadlessSelectorless
				//----------------------------------------
				{
					name: "HeadlessSelectorless_Policy:unset_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:unset_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicySingleStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectError: true,
				}, {
					name: "HeadlessSelectorless_Policy:SingleStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicySingleStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectError: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:PreferDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyPreferDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:unset",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v4v6",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
					expectHeadless: true,
				}, {
					name: "HeadlessSelectorless_Policy:RequireDualStack_Families:v6v4",
					svc: svctest.MakeService("foo",
						svctest.SetHeadless,
						svctest.SetSelector(nil),
						svctest.SetIPFamilyPolicy(api.IPFamilyPolicyRequireDualStack),
						svctest.SetIPFamilies(api.IPv6Protocol, api.IPv4Protocol)),
					expectPolicy:   api.IPFamilyPolicyRequireDualStack,
					expectFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
					expectHeadless: true,
				},
			},
		},
	}

	// This test is ONLY with the gate enabled.
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()

	for _, otc := range testCases {
		t.Run(otc.name, func(t *testing.T) {

			// Do this in the outer loop for performance.
			storage, _, server := newStorage(t, otc.clusterFamilies)
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			for _, itc := range otc.cases {
				t.Run(itc.name, func(t *testing.T) {
					ctx := genericapirequest.NewDefaultContext()
					createdObj, err := storage.Create(ctx, itc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
					if itc.expectError && err != nil {
						return
					}
					if err != nil {
						t.Fatalf("unexpected error creating service: %v", err)
					}
					defer storage.Delete(ctx, itc.svc.Name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
					if itc.expectError && err == nil {
						t.Fatalf("unexpected success creating service")
					}
					createdSvc := createdObj.(*api.Service)

					if want, got := fmtIPFamilyPolicy(&itc.expectPolicy), fmtIPFamilyPolicy(createdSvc.Spec.IPFamilyPolicy); want != got {
						t.Errorf("wrong IPFamilyPolicy: want %s, got %s", want, got)
					}
					if want, got := fmtIPFamilies(itc.expectFamilies), fmtIPFamilies(createdSvc.Spec.IPFamilies); want != got {
						t.Errorf("wrong IPFamilies: want %s, got %s", want, got)
					}
					if itc.expectHeadless {
						proveHeadless(t, storage, nil, createdSvc)
						return
					}
					proveClusterIPsAllocated(t, storage, nil, createdSvc)
				})
			}
		})
	}
}

func TestCreateDeleteReuse(t *testing.T) {
	testCases := []struct {
		name string
		svc  *api.Service
	}{{
		name: "v4",
		svc:  svctest.MakeService("foo", svctest.SetTypeNodePort, svctest.SetIPFamilies(api.IPv4Protocol)),
	}, {
		name: "v6",
		svc:  svctest.MakeService("foo", svctest.SetTypeNodePort, svctest.SetIPFamilies(api.IPv6Protocol)),
	}, {
		name: "v4v6",
		svc:  svctest.MakeService("foo", svctest.SetTypeNodePort, svctest.SetIPFamilies(api.IPv4Protocol, api.IPv6Protocol)),
	}}

	// This test is ONLY with the gate enabled.
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol})
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			ctx := genericapirequest.NewDefaultContext()

			// Create it
			createdObj, err := storage.Create(ctx, tc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}
			createdSvc := createdObj.(*api.Service)

			// Ensure IPs and ports were allocated
			proveClusterIPsAllocated(t, storage, tc.svc, createdSvc)
			proveNodePortsAllocated(t, storage, tc.svc, createdSvc)

			// Delete it
			_, _, err = storage.Delete(ctx, tc.svc.Name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}

			// Ensure IPs and ports were deallocated
			proveClusterIPsDeallocated(t, storage, createdSvc, nil)
			proveNodePortsDeallocated(t, storage, createdSvc, nil)

			// Force the same IPs and ports
			svc2 := tc.svc.DeepCopy()
			svc2.Name += "2"
			svc2.Spec.ClusterIP = createdSvc.Spec.ClusterIP
			svc2.Spec.ClusterIPs = createdSvc.Spec.ClusterIPs
			svc2.Spec.Ports = createdSvc.Spec.Ports

			// Create again
			_, err = storage.Create(ctx, svc2, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}

			// Ensure IPs and ports were allocated
			proveClusterIPsAllocated(t, storage, svc2, createdSvc)
			proveNodePortsAllocated(t, storage, svc2, createdSvc)
		})
	}
}

func TestCreateInitNodePorts(t *testing.T) {
	testCases := []struct {
		name                         string
		svc                          *api.Service
		expectError                  bool
		expectNodePorts              bool
		gateMixedProtocolLBService   bool
		gateServiceLBNodePortControl bool
	}{{
		name:            "type:ExternalName",
		svc:             svctest.MakeService("foo"),
		expectNodePorts: false,
	}, {
		name: "type:ExternalName_with_NodePorts",
		svc: svctest.MakeService("foo",
			svctest.SetUniqueNodePorts),
		expectError: true,
	}, {
		name:            "type:ClusterIP",
		svc:             svctest.MakeService("foo"),
		expectNodePorts: false,
	}, {
		name: "type:ClusterIP_with_NodePorts",
		svc: svctest.MakeService("foo",
			svctest.SetUniqueNodePorts),
		expectError: true,
	}, {
		name: "type:NodePort_single_port_unspecified",
		svc: svctest.MakeService("foo",
			svctest.SetTypeNodePort),
		expectNodePorts: true,
	}, {
		name: "type:NodePort_single_port_specified",
		svc: svctest.MakeService("foo",
			svctest.SetTypeNodePort, svctest.SetUniqueNodePorts),
		expectNodePorts: true,
	}, {
		name: "type:NodePort_multiport_unspecified",
		svc: svctest.MakeService("foo",
			svctest.SetTypeNodePort,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP))),
		expectNodePorts: true,
	}, {
		name: "type:NodePort_multiport_specified",
		svc: svctest.MakeService("foo",
			svctest.SetTypeNodePort,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
			svctest.SetUniqueNodePorts),
		expectNodePorts: true,
	}, {
		name: "type:NodePort_multiport_same",
		svc: svctest.MakeService("foo",
			svctest.SetTypeNodePort,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
			svctest.SetNodePorts(30080, 30080)),
		expectError: true,
	}, {
		name: "type:NodePort_multiport_multiproto_unspecified",
		svc: svctest.MakeService("foo",
			svctest.SetTypeNodePort,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 53, intstr.FromInt(53), api.ProtocolTCP),
				svctest.MakeServicePort("q", 53, intstr.FromInt(53), api.ProtocolUDP))),
		expectNodePorts: true,
	}, {
		name: "type:NodePort_multiport_multiproto_specified",
		svc: svctest.MakeService("foo",
			svctest.SetTypeNodePort,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 53, intstr.FromInt(53), api.ProtocolTCP),
				svctest.MakeServicePort("q", 53, intstr.FromInt(53), api.ProtocolUDP)),
			svctest.SetUniqueNodePorts),
		expectNodePorts: true,
	}, {
		name: "type:NodePort_multiport_multiproto_same",
		svc: svctest.MakeService("foo",
			svctest.SetTypeNodePort,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 53, intstr.FromInt(53), api.ProtocolTCP),
				svctest.MakeServicePort("q", 53, intstr.FromInt(53), api.ProtocolUDP)),
			svctest.SetNodePorts(30053, 30053)),
		expectNodePorts: true,
	}, {
		name: "type:NodePort_multiport_multiproto_conflict",
		svc: svctest.MakeService("foo",
			svctest.SetTypeNodePort,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 93, intstr.FromInt(93), api.ProtocolTCP),
				svctest.MakeServicePort("q", 76, intstr.FromInt(76), api.ProtocolUDP)),
			svctest.SetNodePorts(30093, 30093)),
		expectError: true,
	}, {
		// When the ServiceLBNodePortControl gate is locked, this can be removed.
		name: "type:LoadBalancer_single_port_unspecified_gateServiceLBNodePortControl:off_alloc:nil",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer),
		gateServiceLBNodePortControl: false,
		expectNodePorts:              true,
	}, {
		// When the ServiceLBNodePortControl gate is locked, this can be removed.
		name: "type:LoadBalancer_single_port_unspecified_gateServiceLBNodePortControl:off_alloc:false",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetAllocateLoadBalancerNodePorts(false)),
		gateServiceLBNodePortControl: false,
		expectNodePorts:              true,
	}, {
		// When the ServiceLBNodePortControl gate is locked, this can be removed.
		name: "type:LoadBalancer_single_port_unspecified_gateServiceLBNodePortControl:off_alloc:true",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetAllocateLoadBalancerNodePorts(true)),
		gateServiceLBNodePortControl: false,
		expectNodePorts:              true,
	}, {
		// When the ServiceLBNodePortControl gate is locked, this can be removed.
		name: "type:LoadBalancer_single_port_specified_gateServiceLBNodePortControl:off",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer, svctest.SetUniqueNodePorts),
		gateServiceLBNodePortControl: false,
		expectNodePorts:              true,
	}, {
		// When the ServiceLBNodePortControl gate is locked, this can be removed.
		name: "type:LoadBalancer_multiport_unspecified_gateServiceLBNodePortControl:off",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP))),
		gateServiceLBNodePortControl: false,
		expectNodePorts:              true,
	}, {
		// When the ServiceLBNodePortControl gate is locked, this can be removed.
		name: "type:LoadBalancer_multiport_specified_gateServiceLBNodePortControl:off",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
			svctest.SetUniqueNodePorts),
		gateServiceLBNodePortControl: false,
		expectNodePorts:              true,
	}, {
		name: "type:LoadBalancer_single_port_unspecified_gateServiceLBNodePortControl:on_alloc:false",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetAllocateLoadBalancerNodePorts(false)),
		gateServiceLBNodePortControl: true,
		expectNodePorts:              false,
	}, {
		name: "type:LoadBalancer_single_port_unspecified_gateServiceLBNodePortControl:on_alloc:true",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetAllocateLoadBalancerNodePorts(true)),
		gateServiceLBNodePortControl: true,
		expectNodePorts:              true,
	}, {
		name: "type:LoadBalancer_single_port_specified_gateServiceLBNodePortControl:on_alloc:false",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetUniqueNodePorts,
			svctest.SetAllocateLoadBalancerNodePorts(false)),
		gateServiceLBNodePortControl: true,
		expectNodePorts:              true,
	}, {
		name: "type:LoadBalancer_single_port_specified_gateServiceLBNodePortControl:on_alloc:true",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetUniqueNodePorts,
			svctest.SetAllocateLoadBalancerNodePorts(true)),
		gateServiceLBNodePortControl: true,
		expectNodePorts:              true,
	}, {
		name: "type:LoadBalancer_multiport_unspecified_gateServiceLBNodePortControl:on_alloc:false",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
			svctest.SetAllocateLoadBalancerNodePorts(false)),
		gateServiceLBNodePortControl: true,
		expectNodePorts:              false,
	}, {
		name: "type:LoadBalancer_multiport_unspecified_gateServiceLBNodePortControl:on_alloc:true",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
			svctest.SetAllocateLoadBalancerNodePorts(true)),
		gateServiceLBNodePortControl: true,
		expectNodePorts:              true,
	}, {
		name: "type:LoadBalancer_multiport_specified_gateServiceLBNodePortControl:on_alloc:false",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
			svctest.SetUniqueNodePorts,
			svctest.SetAllocateLoadBalancerNodePorts(false)),
		gateServiceLBNodePortControl: true,
		expectNodePorts:              true,
	}, {
		name: "type:LoadBalancer_multiport_specified_gateServiceLBNodePortControl:on_alloc:true",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
			svctest.SetUniqueNodePorts,
			svctest.SetAllocateLoadBalancerNodePorts(true)),
		gateServiceLBNodePortControl: true,
		expectNodePorts:              true,
	}, {
		name: "type:LoadBalancer_multiport_same",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
			svctest.SetNodePorts(30080, 30080)),
		expectError: true,
	}, {
		// When the MixedProtocolLBService gate is locked, this can be removed.
		name: "type:LoadBalancer_multiport_multiproto_unspecified_MixedProtocolLBService:off",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 53, intstr.FromInt(53), api.ProtocolTCP),
				svctest.MakeServicePort("q", 53, intstr.FromInt(53), api.ProtocolUDP))),
		gateMixedProtocolLBService: false,
		expectError:                true,
	}, {
		// When the MixedProtocolLBService gate is locked, this can be removed.
		name: "type:LoadBalancer_multiport_multiproto_specified_MixedProtocolLBService:off",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 53, intstr.FromInt(53), api.ProtocolTCP),
				svctest.MakeServicePort("q", 53, intstr.FromInt(53), api.ProtocolUDP)),
			svctest.SetUniqueNodePorts),
		gateMixedProtocolLBService: false,
		expectError:                true,
	}, {
		// When the MixedProtocolLBService gate is locked, this can be removed.
		name: "type:LoadBalancer_multiport_multiproto_same_MixedProtocolLBService:off",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 53, intstr.FromInt(53), api.ProtocolTCP),
				svctest.MakeServicePort("q", 53, intstr.FromInt(53), api.ProtocolUDP)),
			svctest.SetNodePorts(30053, 30053)),
		gateMixedProtocolLBService: false,
		expectError:                true,
	}, {
		name: "type:LoadBalancer_multiport_multiproto_unspecified_MixedProtocolLBService:on",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 53, intstr.FromInt(53), api.ProtocolTCP),
				svctest.MakeServicePort("q", 53, intstr.FromInt(53), api.ProtocolUDP))),
		gateMixedProtocolLBService: true,
		expectNodePorts:            true,
	}, {
		name: "type:LoadBalancer_multiport_multiproto_specified_MixedProtocolLBService:on",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 53, intstr.FromInt(53), api.ProtocolTCP),
				svctest.MakeServicePort("q", 53, intstr.FromInt(53), api.ProtocolUDP)),
			svctest.SetUniqueNodePorts),
		gateMixedProtocolLBService: true,
		expectNodePorts:            true,
	}, {
		name: "type:LoadBalancer_multiport_multiproto_same_MixedProtocolLBService:on",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 53, intstr.FromInt(53), api.ProtocolTCP),
				svctest.MakeServicePort("q", 53, intstr.FromInt(53), api.ProtocolUDP)),
			svctest.SetNodePorts(30053, 30053)),
		gateMixedProtocolLBService: true,
		expectNodePorts:            true,
	}, {
		name: "type:LoadBalancer_multiport_multiproto_conflict",
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 93, intstr.FromInt(93), api.ProtocolTCP),
				svctest.MakeServicePort("q", 76, intstr.FromInt(76), api.ProtocolUDP)),
			svctest.SetNodePorts(30093, 30093)),
		expectError: true,
	}}

	// Do this in the outer scope for performance.
	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()

	for _, tc := range testCases {
		defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.ServiceLBNodePortControl, tc.gateServiceLBNodePortControl)()
		defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.MixedProtocolLBService, tc.gateMixedProtocolLBService)()

		t.Run(tc.name, func(t *testing.T) {
			ctx := genericapirequest.NewDefaultContext()
			createdObj, err := storage.Create(ctx, tc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if tc.expectError && err != nil {
				return
			}
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}
			defer storage.Delete(ctx, tc.svc.Name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
			if tc.expectError && err == nil {
				t.Fatalf("unexpected success creating service")
			}
			createdSvc := createdObj.(*api.Service)

			// Produce a map of port index to nodeport value, excluding zero.
			ports := map[int]*api.ServicePort{}
			for i := range createdSvc.Spec.Ports {
				p := &createdSvc.Spec.Ports[i]
				if p.NodePort != 0 {
					ports[i] = p
				}
			}

			if tc.expectNodePorts && len(ports) == 0 {
				t.Fatalf("expected NodePorts to be allocated, found none")
			}
			if !tc.expectNodePorts && len(ports) > 0 {
				t.Fatalf("expected NodePorts to not be allocated, found %v", ports)
			}
			if !tc.expectNodePorts {
				return
			}

			// Make sure we got the right number of allocations
			if want, got := len(ports), len(tc.svc.Spec.Ports); want != got {
				t.Fatalf("expected %d NodePorts, found %d", want, got)
			}

			// Make sure they are all allocated
			for _, p := range ports {
				if !portIsAllocated(t, storage.alloc.serviceNodePorts, p.NodePort) {
					t.Errorf("expected port to be allocated: %v", p)
				}
			}

			// Make sure we got any specific allocations
			for i, p := range tc.svc.Spec.Ports {
				if p.NodePort != 0 {
					if ports[i].NodePort != p.NodePort {
						t.Errorf("expected Ports[%d].NodePort to be %d, got %d", i, p.NodePort, ports[i].NodePort)
					}
					// Remove requested ports from the set
					delete(ports, i)
				}
			}

			// Make sure any allocated ports are unique
			seen := map[int32]int32{}
			for i, p := range ports {
				// We allow the same NodePort for different protocols of the
				// same Port.
				if prev, found := seen[p.NodePort]; found && prev != p.Port {
					t.Errorf("found non-unique allocation in Ports[%d].NodePort: %d -> %d", i, p.NodePort, p.Port)
				}
				seen[p.NodePort] = p.Port
			}
		})
	}
}

// Prove that create skips allocations for Headless services.
func TestCreateSkipsAllocationsForHeadless(t *testing.T) {
	testCases := []struct {
		name            string
		clusterFamilies []api.IPFamily
		enableDualStack bool
		svc             *api.Service
		expectError     bool
	}{{
		name:            "singlestack:v4_gate:off",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		enableDualStack: false,
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "singlestack:v6_gate:on",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol},
		enableDualStack: true,
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "dualstack:v4v6_gate:off",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
		enableDualStack: false,
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "dualstack:v6v4_gate:on",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
		enableDualStack: true,
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "singlestack:v4_gate:off_type:NodePort",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		enableDualStack: false,
		svc:             svctest.MakeService("foo", svctest.SetTypeNodePort),
		expectError:     true,
	}, {
		name:            "singlestack:v6_gate:on_type:LoadBalancer",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol},
		enableDualStack: true,
		svc:             svctest.MakeService("foo", svctest.SetTypeLoadBalancer),
		expectError:     true,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, tc.enableDualStack)()

			storage, _, server := newStorage(t, tc.clusterFamilies)
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			// This test is ONLY headless services.
			tc.svc.Spec.ClusterIP = api.ClusterIPNone

			ctx := genericapirequest.NewDefaultContext()
			createdObj, err := storage.Create(ctx, tc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if tc.expectError && err != nil {
				return
			}
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}
			if tc.expectError && err == nil {
				t.Fatalf("unexpected success creating service")
			}
			createdSvc := createdObj.(*api.Service)

			if createdSvc.Spec.ClusterIP != "None" {
				t.Errorf("expected clusterIP \"None\", got %q", createdSvc.Spec.ClusterIP)
			}
			if !reflect.DeepEqual(createdSvc.Spec.ClusterIPs, []string{"None"}) {
				t.Errorf("expected clusterIPs [\"None\"], got %q", createdSvc.Spec.ClusterIPs)
			}
		})
	}
}

// Prove that a dry-run create doesn't actually allocate IPs or ports.
func TestCreateDryRun(t *testing.T) {
	testCases := []struct {
		name            string
		clusterFamilies []api.IPFamily
		enableDualStack bool
		svc             *api.Service
	}{{
		name:            "singlestack:v4_gate:off_clusterip:unset",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		enableDualStack: false,
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "singlestack:v4_gate:off_clusterip:set",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		enableDualStack: false,
		svc:             svctest.MakeService("foo", svctest.SetClusterIPs("10.0.0.1")),
	}, {
		name:            "singlestack:v6_gate:on_clusterip:unset",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol},
		enableDualStack: true,
		svc:             svctest.MakeService("foo"),
	}, {
		name:            "singlestack:v6_gate:on_clusterip:set",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol},
		enableDualStack: true,
		svc:             svctest.MakeService("foo", svctest.SetClusterIPs("2000::1")),
	}, {
		name:            "dualstack:v4v6_gate:on_clusterip:unset",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
		enableDualStack: true,
		svc:             svctest.MakeService("foo", svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack)),
	}, {
		name:            "dualstack:v4v6_gate:on_clusterip:set",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
		enableDualStack: true,
		svc:             svctest.MakeService("foo", svctest.SetIPFamilyPolicy(api.IPFamilyPolicyPreferDualStack), svctest.SetClusterIPs("10.0.0.1", "2000::1")),
	}, {
		name:            "singlestack:v4_gate:off_type:NodePort_nodeport:unset",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		enableDualStack: false,
		svc:             svctest.MakeService("foo", svctest.SetTypeNodePort),
	}, {
		name:            "singlestack:v4_gate:on_type:LoadBalancer_nodePort:set",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		enableDualStack: true,
		svc:             svctest.MakeService("foo", svctest.SetTypeLoadBalancer, svctest.SetUniqueNodePorts),
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, tc.enableDualStack)()

			storage, _, server := newStorage(t, tc.clusterFamilies)
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			ctx := genericapirequest.NewDefaultContext()
			createdObj, err := storage.Create(ctx, tc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}
			createdSvc := createdObj.(*api.Service)

			// Ensure IPs were assigned
			if net.ParseIP(createdSvc.Spec.ClusterIP) == nil {
				t.Errorf("expected valid clusterIP: %q", createdSvc.Spec.ClusterIP)
			}
			for _, ip := range createdSvc.Spec.ClusterIPs {
				if net.ParseIP(ip) == nil {
					t.Errorf("expected valid clusterIP: %v", createdSvc.Spec.ClusterIP)
				}
			}

			// Ensure the allocators are clean.
			proveClusterIPsDeallocated(t, storage, createdSvc, nil)
			if tc.svc.Spec.Type != api.ServiceTypeClusterIP {
				proveNodePortsDeallocated(t, storage, createdSvc, nil)
			}
		})
	}
}

func TestDeleteWithFinalizer(t *testing.T) {
	svcName := "foo"

	// This test is ONLY with the gate enabled.
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()

	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()

	// This will allocate cluster IPs, NodePort, and HealthCheckNodePort.
	svc := svctest.MakeService(svcName, svctest.SetTypeLoadBalancer,
		svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
		func(s *api.Service) {
			s.Finalizers = []string{"example.com/test"}
		})

	ctx := genericapirequest.NewDefaultContext()

	// Create it with finalizer.
	obj, err := storage.Create(ctx, svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unexpected error creating service: %v", err)
	}
	createdSvc := obj.(*api.Service)

	// Prove everything was allocated.
	obj, err = storage.Get(ctx, svcName, &metav1.GetOptions{})
	if err != nil {
		t.Fatalf("unexpected error getting service: %v", err)
	}
	if !cmp.Equal(createdSvc, obj) {
		t.Errorf("expected the result of Create() and Get() to match: %v", cmp.Diff(createdSvc, obj))
	}
	proveClusterIPsAllocated(t, storage, svc, createdSvc)
	proveNodePortsAllocated(t, storage, svc, createdSvc)
	proveHealthCheckNodePortAllocated(t, storage, svc, createdSvc)

	// Try to delete it, but it should be blocked by the finalizer.
	obj, deleted, err := storage.Delete(ctx, svcName, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("unexpected error deleting service: %v", err)
	}
	if deleted {
		t.Fatalf("expected service to not be deleted")
	}
	deletedSvc := obj.(*api.Service)

	// Prove everything is still allocated.
	_, err = storage.Get(ctx, svcName, &metav1.GetOptions{})
	if err != nil {
		t.Fatalf("unexpected error getting service: %v", err)
	}
	proveClusterIPsAllocated(t, storage, svc, createdSvc)
	proveNodePortsAllocated(t, storage, svc, createdSvc)
	proveHealthCheckNodePortAllocated(t, storage, svc, createdSvc)

	// Clear the finalizer - should delete.
	deletedSvc.Finalizers = nil
	_, _, err = storage.Update(ctx, svcName,
		rest.DefaultUpdatedObjectInfo(deletedSvc), rest.ValidateAllObjectFunc,
		rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("unexpected error updating service: %v", err)
	}

	// Prove everything is deallocated.
	_, err = storage.Get(ctx, svcName, &metav1.GetOptions{})
	if err == nil {
		t.Fatalf("unexpected success getting service")
	}
	proveClusterIPsDeallocated(t, storage, createdSvc, nil)
	proveNodePortsDeallocated(t, storage, createdSvc, nil)
	proveHealthCheckNodePortDeallocated(t, storage, createdSvc, nil)
}

// Prove that a dry-run delete doesn't actually deallocate IPs or ports.
func TestDeleteDryRun(t *testing.T) {
	testCases := []struct {
		name            string
		enableDualStack bool
		svc             *api.Service
	}{{
		name:            "gate:off",
		enableDualStack: false,
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
	}, {
		name:            "gate:on",
		enableDualStack: true,
		svc: svctest.MakeService("foo",
			svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, tc.enableDualStack)()

			families := []api.IPFamily{api.IPv4Protocol}
			if tc.enableDualStack {
				families = append(families, api.IPv6Protocol)
			}

			storage, _, server := newStorage(t, families)
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			ctx := genericapirequest.NewDefaultContext()
			createdObj, err := storage.Create(ctx, tc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}
			createdSvc := createdObj.(*api.Service)

			// Ensure IPs and ports were allocated
			proveClusterIPsAllocated(t, storage, tc.svc, createdSvc)
			proveNodePortsAllocated(t, storage, tc.svc, createdSvc)
			proveHealthCheckNodePortAllocated(t, storage, tc.svc, createdSvc)

			_, _, err = storage.Delete(ctx, tc.svc.Name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}})
			if err != nil {
				t.Fatalf("unexpected error deleting service: %v", err)
			}

			// Ensure they are still allocated.
			proveClusterIPsAllocated(t, storage, tc.svc, createdSvc)
			proveNodePortsAllocated(t, storage, tc.svc, createdSvc)
			proveHealthCheckNodePortAllocated(t, storage, tc.svc, createdSvc)
		})
	}
}

// Prove that a dry-run update doesn't actually allocate or deallocate IPs or ports.
func TestUpdateDryRun(t *testing.T) {
	testCases := []struct {
		name            string
		clusterFamilies []api.IPFamily
		svc             *api.Service
		update          *api.Service
		verifyDryAllocs bool
	}{{
		name:            "singlestack:v4_NoAllocs-Allocs",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		svc:             svctest.MakeService("foo", svctest.SetTypeExternalName),
		update: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
		verifyDryAllocs: true, // make sure values were not allocated.
	}, {
		name:            "singlestack:v4_Allocs-NoAllocs",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol},
		svc: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
		update:          svctest.MakeService("foo", svctest.SetTypeExternalName),
		verifyDryAllocs: false, // make sure values were not released.
	}, {
		name:            "singlestack:v6_NoAllocs-Allocs",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol},
		svc:             svctest.MakeService("foo", svctest.SetTypeExternalName),
		update: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
		verifyDryAllocs: true, // make sure values were not allocated.
	}, {
		name:            "singlestack:v6_Allocs-NoAllocs",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol},
		svc: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
		update:          svctest.MakeService("foo", svctest.SetTypeExternalName),
		verifyDryAllocs: false, // make sure values were not released.
	}, {
		name:            "dualstack:v4v6_NoAllocs-Allocs",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
		svc:             svctest.MakeService("foo", svctest.SetTypeExternalName),
		update: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
		verifyDryAllocs: true, // make sure values were not allocated.
	}, {
		name:            "dualstack:v4v6_Allocs-NoAllocs",
		clusterFamilies: []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol},
		svc: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
		update:          svctest.MakeService("foo", svctest.SetTypeExternalName),
		verifyDryAllocs: false, // make sure values were not released.
	}, {
		name:            "dualstack:v6v4_NoAllocs-Allocs",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
		svc:             svctest.MakeService("foo", svctest.SetTypeExternalName),
		update: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
		verifyDryAllocs: true, // make sure values were not allocated.
	}, {
		name:            "dualstack:v6v4_Allocs-NoAllocs",
		clusterFamilies: []api.IPFamily{api.IPv6Protocol, api.IPv4Protocol},
		svc: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
		update:          svctest.MakeService("foo", svctest.SetTypeExternalName),
		verifyDryAllocs: false, // make sure values were not released.
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			storage, _, server := newStorage(t, tc.clusterFamilies)
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			ctx := genericapirequest.NewDefaultContext()
			obj, err := storage.Create(ctx, tc.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}
			createdSvc := obj.(*api.Service)

			if tc.verifyDryAllocs {
				// Dry allocs means no allocs on create.  Ensure values were
				// NOT allocated.
				proveClusterIPsDeallocated(t, storage, nil, createdSvc)
			} else {
				// Ensure IPs were allocated
				proveClusterIPsAllocated(t, storage, nil, createdSvc)
			}

			// Update the object to the new state and check the results.
			obj, _, err = storage.Update(ctx, tc.update.Name,
				rest.DefaultUpdatedObjectInfo(tc.update), rest.ValidateAllObjectFunc,
				rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}})
			if err != nil {
				t.Fatalf("unexpected error updating service: %v", err)
			}
			updatedSvc := obj.(*api.Service)

			if tc.verifyDryAllocs {
				// Dry allocs means the values are assigned but not
				// allocated.
				if net.ParseIP(updatedSvc.Spec.ClusterIP) == nil {
					t.Errorf("expected valid clusterIP: %q", updatedSvc.Spec.ClusterIP)
				}
				for _, ip := range updatedSvc.Spec.ClusterIPs {
					if net.ParseIP(ip) == nil {
						t.Errorf("expected valid clusterIP: %q", updatedSvc.Spec.ClusterIP)
					}
				}
				for i, fam := range updatedSvc.Spec.IPFamilies {
					if ipIsAllocated(t, storage.alloc.serviceIPAllocatorsByFamily[fam], updatedSvc.Spec.ClusterIPs[i]) {
						t.Errorf("expected IP to not be allocated: %q", updatedSvc.Spec.ClusterIPs[i])
					}
				}

				for _, p := range updatedSvc.Spec.Ports {
					if p.NodePort == 0 {
						t.Errorf("expected nodePort to be assigned: %d", p.NodePort)
					}
					if portIsAllocated(t, storage.alloc.serviceNodePorts, p.NodePort) {
						t.Errorf("expected nodePort to not be allocated: %d", p.NodePort)
					}
				}

				if updatedSvc.Spec.HealthCheckNodePort == 0 {
					t.Errorf("expected HCNP to be assigned: %d", updatedSvc.Spec.HealthCheckNodePort)
				}
				if portIsAllocated(t, storage.alloc.serviceNodePorts, updatedSvc.Spec.HealthCheckNodePort) {
					t.Errorf("expected HCNP to not be allocated: %d", updatedSvc.Spec.HealthCheckNodePort)
				}
			} else {
				// Ensure IPs were unassigned but not deallocated.
				if updatedSvc.Spec.ClusterIP != "" {
					t.Errorf("expected clusterIP to be unset: %q", updatedSvc.Spec.ClusterIP)
				}
				if len(updatedSvc.Spec.ClusterIPs) != 0 {
					t.Errorf("expected clusterIPs to be unset: %q", updatedSvc.Spec.ClusterIPs)
				}
				for i, fam := range createdSvc.Spec.IPFamilies {
					if !ipIsAllocated(t, storage.alloc.serviceIPAllocatorsByFamily[fam], createdSvc.Spec.ClusterIPs[i]) {
						t.Errorf("expected IP to still be allocated: %q", createdSvc.Spec.ClusterIPs[i])
					}
				}

				for _, p := range updatedSvc.Spec.Ports {
					if p.NodePort != 0 {
						t.Errorf("expected nodePort to be unset: %d", p.NodePort)
					}
				}
				for _, p := range createdSvc.Spec.Ports {
					if !portIsAllocated(t, storage.alloc.serviceNodePorts, p.NodePort) {
						t.Errorf("expected nodePort to still be allocated: %d", p.NodePort)
					}
				}

				if updatedSvc.Spec.HealthCheckNodePort != 0 {
					t.Errorf("expected HCNP to be unset: %d", updatedSvc.Spec.HealthCheckNodePort)
				}
				if !portIsAllocated(t, storage.alloc.serviceNodePorts, createdSvc.Spec.HealthCheckNodePort) {
					t.Errorf("expected HCNP to still be allocated: %d", createdSvc.Spec.HealthCheckNodePort)
				}
			}
		})
	}
}

//FIXME: after all the tests are ported, move this all up near the top?
type svcTestCase struct {
	svc         *api.Service
	expectError bool

	// We could calculate these by looking at the Service, but that's a
	// vector for test bugs and more importantly it makes the test cases less
	// self-documenting.
	expectClusterIPs          bool
	expectHeadless            bool
	expectNodePorts           bool
	expectHealthCheckNodePort bool
}

func callName(before, after *api.Service) string {
	if before == nil && after != nil {
		return "create"
	}
	if before != nil && after != nil {
		return "update"
	}
	if before != nil && after == nil {
		return "delete"
	}
	panic("this test is broken: before and after are both nil")
}

func proveClusterIPsAllocated(t *testing.T, storage *GenericREST, before, after *api.Service) {
	t.Helper()

	if utilfeature.DefaultFeatureGate.Enabled(features.IPv6DualStack) {
		if sing, plur := after.Spec.ClusterIP, after.Spec.ClusterIPs[0]; sing != plur {
			t.Errorf("%s: expected clusterIP == clusterIPs[0]: %q != %q", callName(before, after), sing, plur)
		}
	}

	clips := []string{}
	if utilfeature.DefaultFeatureGate.Enabled(features.IPv6DualStack) {
		clips = after.Spec.ClusterIPs
	} else {
		clips = append(clips, after.Spec.ClusterIP)
	}
	for _, clip := range clips {
		if !ipIsAllocated(t, storage.alloc.serviceIPAllocatorsByFamily[familyOf(clip)], clip) {
			t.Errorf("%s: expected clusterIP to be allocated: %q", callName(before, after), clip)
		}
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.IPv6DualStack) {
		if lc, lf := len(after.Spec.ClusterIPs), len(after.Spec.IPFamilies); lc != lf {
			t.Errorf("%s: expected same number of clusterIPs and ipFamilies: %d != %d", callName(before, after), lc, lf)
		}
	}

	for i, fam := range after.Spec.IPFamilies {
		if want, got := fam, familyOf(after.Spec.ClusterIPs[i]); want != got {
			t.Errorf("%s: clusterIP is the wrong IP family: want %s, got %s", callName(before, after), want, got)
		}
	}

	if before != nil {
		if before.Spec.ClusterIP != "" {
			if want, got := before.Spec.ClusterIP, after.Spec.ClusterIP; want != got {
				t.Errorf("%s: wrong clusterIP: wanted %q, got %q", callName(before, after), want, got)
			}
		}
		for i := range before.Spec.ClusterIPs {
			if want, got := before.Spec.ClusterIPs[i], after.Spec.ClusterIPs[i]; want != got {
				t.Errorf("%s: wrong clusterIPs[%d]: wanted %q, got %q", callName(before, after), i, want, got)
			}
		}
		for i := range before.Spec.IPFamilies {
			if want, got := before.Spec.IPFamilies[i], after.Spec.IPFamilies[i]; want != got {
				t.Errorf("%s: wrong IPFamilies[%d]: wanted %q, got %q", callName(before, after), i, want, got)
			}
		}
	}
}

func proveClusterIPsDeallocated(t *testing.T, storage *GenericREST, before, after *api.Service) {
	t.Helper()

	if after != nil && after.Spec.ClusterIP != api.ClusterIPNone {
		if after.Spec.ClusterIP != "" {
			t.Errorf("%s: expected clusterIP to be unset: %q", callName(before, after), after.Spec.ClusterIP)
		}
		if len(after.Spec.ClusterIPs) != 0 {
			t.Errorf("%s: expected clusterIPs to be unset: %q", callName(before, after), after.Spec.ClusterIPs)
		}
	}

	if before != nil && before.Spec.ClusterIP != api.ClusterIPNone {
		clips := []string{}
		if utilfeature.DefaultFeatureGate.Enabled(features.IPv6DualStack) {
			clips = before.Spec.ClusterIPs
		} else {
			clips = append(clips, before.Spec.ClusterIP)
		}
		for _, clip := range clips {
			if ipIsAllocated(t, storage.alloc.serviceIPAllocatorsByFamily[familyOf(clip)], clip) {
				t.Errorf("%s: expected clusterIP to be deallocated: %q", callName(before, after), clip)
			}
		}
	}
}

func proveHeadless(t *testing.T, storage *GenericREST, before, after *api.Service) {
	t.Helper()

	if sing, plur := after.Spec.ClusterIP, after.Spec.ClusterIPs[0]; sing != plur {
		t.Errorf("%s: expected clusterIP == clusterIPs[0]: %q != %q", callName(before, after), sing, plur)
	}
	if len(after.Spec.ClusterIPs) != 1 || after.Spec.ClusterIPs[0] != api.ClusterIPNone {
		t.Errorf("%s: expected clusterIPs to be [%q]: %q", callName(before, after), api.ClusterIPNone, after.Spec.ClusterIPs)
	}
}

func proveNodePortsAllocated(t *testing.T, storage *GenericREST, before, after *api.Service) {
	t.Helper()

	for _, p := range after.Spec.Ports {
		if !portIsAllocated(t, storage.alloc.serviceNodePorts, p.NodePort) {
			t.Errorf("%s: expected nodePort to be allocated: %d", callName(before, after), p.NodePort)
		}
	}

	if before != nil {
		for i, p := range before.Spec.Ports {
			if p.NodePort == 0 {
				continue
			}
			if want, got := p.NodePort, after.Spec.Ports[i].NodePort; want != got {
				t.Errorf("%s: wrong ports[%d].nodePort value: wanted %d, got %d", callName(before, after), i, want, got)
			}
		}
	}
}

func proveNodePortsDeallocated(t *testing.T, storage *GenericREST, before, after *api.Service) {
	t.Helper()

	if after != nil {
		for _, p := range after.Spec.Ports {
			if p.NodePort != 0 {
				t.Errorf("%s: expected nodePort to be unset: %d", callName(before, after), p.NodePort)
			}
		}
	}

	if before != nil {
		for _, p := range before.Spec.Ports {
			if p.NodePort != 0 && portIsAllocated(t, storage.alloc.serviceNodePorts, p.NodePort) {
				t.Errorf("%s: expected nodePort to be deallocated: %d", callName(before, after), p.NodePort)
			}
		}
	}
}

func proveHealthCheckNodePortAllocated(t *testing.T, storage *GenericREST, before, after *api.Service) {
	t.Helper()

	if !portIsAllocated(t, storage.alloc.serviceNodePorts, after.Spec.HealthCheckNodePort) {
		t.Errorf("%s: expected healthCheckNodePort to be allocated: %d", callName(before, after), after.Spec.HealthCheckNodePort)
	}

	if before != nil {
		if before.Spec.HealthCheckNodePort != 0 {
			if want, got := before.Spec.HealthCheckNodePort, after.Spec.HealthCheckNodePort; want != got {
				t.Errorf("%s: wrong healthCheckNodePort value: wanted %d, got %d", callName(before, after), want, got)
			}
		}
	}
}

func proveHealthCheckNodePortDeallocated(t *testing.T, storage *GenericREST, before, after *api.Service) {
	t.Helper()

	if after != nil {
		if after.Spec.HealthCheckNodePort != 0 {
			t.Errorf("%s: expected healthCheckNodePort to be unset: %d", callName(before, after), after.Spec.HealthCheckNodePort)
		}
	}

	if before != nil {
		if before.Spec.HealthCheckNodePort != 0 && portIsAllocated(t, storage.alloc.serviceNodePorts, before.Spec.HealthCheckNodePort) {
			t.Errorf("%s: expected healthCheckNodePort to be deallocated: %d", callName(before, after), before.Spec.HealthCheckNodePort)
		}
	}
}

type cudTestCase struct {
	name   string
	create svcTestCase
	update svcTestCase
}

func helpTestCreateUpdateDelete(t *testing.T, testCases []cudTestCase) {
	// NOTE: do not call t.Helper() here.  It's more useful for errors to be
	// attributed to lines in this function than the caller of it.

	// This test is ONLY with the gate enabled.
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()

	storage, _, server := newStorage(t, []api.IPFamily{api.IPv4Protocol, api.IPv6Protocol})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := genericapirequest.NewDefaultContext()

			// Create the object as specified and check the results.
			obj, err := storage.Create(ctx, tc.create.svc, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if tc.create.expectError && err != nil {
				return
			}
			if err != nil {
				t.Fatalf("unexpected error creating service: %v", err)
			}
			defer storage.Delete(ctx, tc.create.svc.Name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{}) // in case
			if tc.create.expectError && err == nil {
				t.Fatalf("unexpected success creating service")
			}
			createdSvc := obj.(*api.Service)
			if !verifyEquiv(t, "create", &tc.create, createdSvc) {
				return
			}
			verifyExpectations(t, storage, tc.create, tc.create.svc, createdSvc)

			// Update the object to the new state and check the results.
			obj, created, err := storage.Update(ctx, tc.update.svc.Name,
				rest.DefaultUpdatedObjectInfo(tc.update.svc), rest.ValidateAllObjectFunc,
				rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
			if tc.update.expectError && err != nil {
				return
			}
			if err != nil {
				t.Fatalf("unexpected error updating service: %v", err)
			}
			if tc.update.expectError && err == nil {
				t.Fatalf("unexpected success updating service")
			}
			if created {
				t.Fatalf("unexpected create-on-update")
			}
			updatedSvc := obj.(*api.Service)
			if !verifyEquiv(t, "update", &tc.update, updatedSvc) {
				return
			}
			verifyExpectations(t, storage, tc.update, createdSvc, updatedSvc)

			// Delete the object and check the results.
			_, _, err = storage.Delete(ctx, tc.update.svc.Name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
			if err != nil {
				t.Fatalf("unexpected error deleting service: %v", err)
			}
			verifyExpectations(t, storage, svcTestCase{ /* all false */ }, updatedSvc, nil)
		})
	}
}

type testingTInterface interface {
	Helper()
	Errorf(format string, args ...interface{})
}

type fakeTestingT struct {
	t *testing.T
}

func (f fakeTestingT) Helper() {}

func (f fakeTestingT) Errorf(format string, args ...interface{}) {
	f.t.Logf(format, args...)
}

func verifyEquiv(t testingTInterface, call string, tc *svcTestCase, got *api.Service) bool {
	t.Helper()

	// For when we compare objects.
	options := []cmp.Option{
		// These are system-assigned values, we don't need to compare them.
		cmpopts.IgnoreFields(api.Service{}, "UID", "ResourceVersion", "CreationTimestamp"),
		// Treat nil slices and empty slices as the same (e.g. clusterIPs).
		cmpopts.EquateEmpty(),
	}

	// For allocated fields, we want to be able to compare cleanly whether the
	// input specified values or not.
	want := tc.svc.DeepCopy()
	if tc.expectClusterIPs || tc.expectHeadless {
		if want.Spec.ClusterIP == "" {
			want.Spec.ClusterIP = got.Spec.ClusterIP
		}
		if utilfeature.DefaultFeatureGate.Enabled(features.IPv6DualStack) {
			if len(got.Spec.ClusterIPs) > len(want.Spec.ClusterIPs) {
				want.Spec.ClusterIPs = append(want.Spec.ClusterIPs, got.Spec.ClusterIPs[len(want.Spec.ClusterIPs):]...)
			}
			if want.Spec.IPFamilyPolicy == nil {
				want.Spec.IPFamilyPolicy = got.Spec.IPFamilyPolicy
			}
			if len(got.Spec.IPFamilies) > len(want.Spec.IPFamilies) {
				want.Spec.IPFamilies = append(want.Spec.IPFamilies, got.Spec.IPFamilies[len(want.Spec.IPFamilies):]...)
			}
		}
	}
	if tc.expectNodePorts {
		for i := range want.Spec.Ports {
			p := &want.Spec.Ports[i]
			if p.NodePort == 0 {
				p.NodePort = got.Spec.Ports[i].NodePort
			}
		}
	}
	if tc.expectHealthCheckNodePort {
		if want.Spec.HealthCheckNodePort == 0 {
			want.Spec.HealthCheckNodePort = got.Spec.HealthCheckNodePort
		}
	}

	if !cmp.Equal(want, got, options...) {
		t.Errorf("unexpected result from %s:\n%s", call, cmp.Diff(want, got, options...))
		return false
	}
	return true
}

// Quis custodiet ipsos custodes?
func TestVerifyEquiv(t *testing.T) {
	testCases := []struct {
		name   string
		input  svcTestCase
		output *api.Service
		expect bool
	}{{
		name: "ExternalName",
		input: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
		output: svctest.MakeService("foo", svctest.SetTypeExternalName),
		expect: true,
	}, {
		name: "ClusterIPs_unspecified",
		input: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeClusterIP, svctest.SetClusterIPs("10.0.0.1", "2000:1")),
		expect: true,
	}, {
		name: "ClusterIPs_specified",
		input: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP, svctest.SetClusterIPs("10.0.0.1", "2000:1")),
			expectClusterIPs: true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeClusterIP, svctest.SetClusterIPs("10.0.0.1", "2000:1")),
		expect: true,
	}, {
		name: "ClusterIPs_wrong",
		input: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP, svctest.SetClusterIPs("10.0.0.0", "2000:0")),
			expectClusterIPs: true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeClusterIP, svctest.SetClusterIPs("10.0.0.1", "2000:1")),
		expect: false,
	}, {
		name: "ClusterIPs_partial",
		input: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP, svctest.SetClusterIPs("10.0.0.1")),
			expectClusterIPs: true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeClusterIP, svctest.SetClusterIPs("10.0.0.1", "2000:1")),
		expect: true,
	}, {
		name: "NodePort_unspecified",
		input: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeNodePort, svctest.SetUniqueNodePorts),
		expect: true,
	}, {
		name: "NodePort_specified",
		input: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort, svctest.SetNodePorts(93)),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeNodePort, svctest.SetNodePorts(93)),
		expect: true,
	}, {
		name: "NodePort_wrong",
		input: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort, svctest.SetNodePorts(93)),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeNodePort, svctest.SetNodePorts(76)),
		expect: false,
	}, {
		name: "NodePort_partial",
		input: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeNodePort,
				svctest.SetPorts(
					svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
					svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
				svctest.SetNodePorts(93)),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeNodePort,
			svctest.SetPorts(
				svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
				svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP)),
			svctest.SetNodePorts(93, 76)),
		expect: true,
	}, {
		name: "HealthCheckNodePort_unspecified",
		input: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
			svctest.SetHealthCheckNodePort(93)),
		expect: true,
	}, {
		name: "HealthCheckNodePort_specified",
		input: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(93)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
			svctest.SetHealthCheckNodePort(93)),
		expect: true,
	}, {
		name: "HealthCheckNodePort_wrong",
		input: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(93)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: true,
		},
		output: svctest.MakeService("foo", svctest.SetTypeLoadBalancer,
			svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
			svctest.SetHealthCheckNodePort(76)),
		expect: false,
	}}

	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := verifyEquiv(fakeTestingT{t}, "test", &tc.input, tc.output)
			if result != tc.expect {
				t.Errorf("expected %v, got %v", tc.expect, result)
			}
		})
	}
}

func verifyExpectations(t *testing.T, storage *GenericREST, tc svcTestCase, before, after *api.Service) {
	t.Helper()

	if tc.expectClusterIPs {
		proveClusterIPsAllocated(t, storage, before, after)
	} else if tc.expectHeadless {
		proveHeadless(t, storage, before, after)
	} else {
		proveClusterIPsDeallocated(t, storage, before, after)
	}
	if tc.expectNodePorts {
		proveNodePortsAllocated(t, storage, before, after)
	} else {
		proveNodePortsDeallocated(t, storage, before, after)
	}
	if tc.expectHealthCheckNodePort {
		proveHealthCheckNodePortAllocated(t, storage, before, after)
	} else {
		proveHealthCheckNodePortDeallocated(t, storage, before, after)
	}
}

func TestFeatureExternalName(t *testing.T) {
	testCases := []cudTestCase{{
		name: "valid-valid",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName, svctest.SetExternalName("updated.example.com")),
		},
	}, {
		name: "valid-blank",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
		update: svcTestCase{
			svc:         svctest.MakeService("foo", svctest.SetTypeExternalName, svctest.SetExternalName("")),
			expectError: true,
		},
	}}

	helpTestCreateUpdateDelete(t, testCases)
}

func TestFeatureSelector(t *testing.T) {
	testCases := []cudTestCase{{
		name: "valid-valid",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				func(s *api.Service) {
					s.Spec.Selector = map[string]string{"updated": "value"}
				}),
			expectClusterIPs: true,
		},
	}, {
		name: "valid-nil",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				func(s *api.Service) {
					s.Spec.Selector = nil
				}),
			expectClusterIPs: true,
		},
	}, {
		name: "valid-empty",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				func(s *api.Service) {
					s.Spec.Selector = map[string]string{}
				}),
			expectClusterIPs: true,
		},
	}, {
		name: "nil-valid",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				func(s *api.Service) {
					s.Spec.Selector = nil
				}),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
	}, {
		name: "empty-valid",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				func(s *api.Service) {
					s.Spec.Selector = map[string]string{}
				}),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
	}}

	helpTestCreateUpdateDelete(t, testCases)
}

func TestFeatureClusterIPs(t *testing.T) {
	testCases := []cudTestCase{{
		name: "clusterIP:valid-headless",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetHeadless),
			expectError: true,
		},
	}, {
		name: "clusterIP:headless-valid",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetHeadless),
			expectHeadless: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetClusterIP("10.0.0.93")),
			expectError: true,
		},
	}, {
		name: "clusterIP:valid-valid",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetClusterIP("10.0.0.93")),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetClusterIP("10.0.0.76")),
			expectError: true,
		},
	}, {
		name: "clusterIPs:valid-valid",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetClusterIPs("10.0.0.93", "2000::93")),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetClusterIPs("10.0.0.76", "2000::76")),
			expectError: true,
		},
	}}

	helpTestCreateUpdateDelete(t, testCases)
}

func TestFeaturePorts(t *testing.T) {
	testCases := []cudTestCase{{
		name: "add",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts(
					svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP))),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts(
					svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
					svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP))),
			expectClusterIPs: true,
		},
	}, {
		name: "remove",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts(
					svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
					svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP))),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts(
					svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP))),
			expectClusterIPs: true,
		},
	}, {
		name: "swap",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts(
					svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
					svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP))),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts(
					svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP),
					svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP))),
			expectClusterIPs: true,
		},
	}, {
		name: "modify",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts(
					svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
					svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP))),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts(
					svctest.MakeServicePort("r", 53, intstr.FromInt(53), api.ProtocolTCP),
					svctest.MakeServicePort("s", 53, intstr.FromInt(53), api.ProtocolUDP))),
			expectClusterIPs: true,
		},
	}, {
		name: "wipe",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts(
					svctest.MakeServicePort("p", 80, intstr.FromInt(80), api.ProtocolTCP),
					svctest.MakeServicePort("q", 443, intstr.FromInt(443), api.ProtocolTCP))),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetPorts()),
			expectError: true,
		},
	}}

	helpTestCreateUpdateDelete(t, testCases)
}

func TestFeatureSessionAffinity(t *testing.T) {
	testCases := []cudTestCase{{
		name: "None-ClientIPNoConfig",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetSessionAffinity(api.ServiceAffinityNone)),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				func(s *api.Service) {
					// Set it without setting the config
					s.Spec.SessionAffinity = api.ServiceAffinityClientIP
				}),
			expectError: true,
		},
	}, {
		name: "None-ClientIP",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetSessionAffinity(api.ServiceAffinityNone)),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetSessionAffinity(api.ServiceAffinityClientIP)),
			expectClusterIPs: true,
		},
	}, {
		name: "ClientIP-NoneWithConfig",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetSessionAffinity(api.ServiceAffinityClientIP)),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetSessionAffinity(api.ServiceAffinityClientIP),
				func(s *api.Service) {
					// Set it without wiping the config
					s.Spec.SessionAffinity = api.ServiceAffinityNone
				}),
			expectError: true,
		},
	}, {
		name: "ClientIP-None",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetSessionAffinity(api.ServiceAffinityClientIP)),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeClusterIP,
				svctest.SetSessionAffinity(api.ServiceAffinityNone),
				func(s *api.Service) {
					s.Spec.SessionAffinityConfig = nil
				}),
			expectClusterIPs: true,
		},
	}}

	helpTestCreateUpdateDelete(t, testCases)
}

func TestFeatureType(t *testing.T) {
	testCases := []cudTestCase{{
		name: "ExternalName-ClusterIP",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
	}, {
		name: "ClusterIP-ExternalName",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
	}, {
		name: "ExternalName-NodePort",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
	}, {
		name: "NodePort-ExternalName",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
	}, {
		name: "ExternalName-LoadBalancer",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeLoadBalancer),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
	}, {
		name: "LoadBalancer-ExternalName",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeLoadBalancer),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
	}, {
		name: "ClusterIP-NodePort",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
	}, {
		name: "NodePort-ClusterIP",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
	}, {
		name: "ClusterIP-LoadBalancer",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeLoadBalancer),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
	}, {
		name: "LoadBalancer-ClusterIP",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeLoadBalancer),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeClusterIP),
			expectClusterIPs: true,
		},
	}, {
		name: "NodePort-LoadBalancer",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeLoadBalancer),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
	}, {
		name: "LoadBalancer-NodePort",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeLoadBalancer),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		update: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
	}, {
		name: "Headless-ExternalName",
		create: svcTestCase{
			svc:            svctest.MakeService("foo", svctest.SetHeadless),
			expectHeadless: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
	}, {
		name: "ExternalName-Headless",
		create: svcTestCase{
			svc: svctest.MakeService("foo", svctest.SetTypeExternalName),
		},
		update: svcTestCase{
			svc:            svctest.MakeService("foo", svctest.SetHeadless),
			expectHeadless: true,
		},
	}, {
		name: "Headless-NodePort",
		create: svcTestCase{
			svc:            svctest.MakeService("foo", svctest.SetHeadless),
			expectHeadless: true,
		},
		update: svcTestCase{
			svc:         svctest.MakeService("foo", svctest.SetTypeNodePort),
			expectError: true,
		},
	}, {
		name: "NodePort-Headless",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeNodePort),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		update: svcTestCase{
			svc:         svctest.MakeService("foo", svctest.SetHeadless),
			expectError: true,
		},
	}, {
		name: "Headless-LoadBalancer",
		create: svcTestCase{
			svc:            svctest.MakeService("foo", svctest.SetHeadless),
			expectHeadless: true,
		},
		update: svcTestCase{
			svc:         svctest.MakeService("foo", svctest.SetTypeLoadBalancer),
			expectError: true,
		},
	}, {
		name: "LoadBalancer-Headless",
		create: svcTestCase{
			svc:              svctest.MakeService("foo", svctest.SetTypeLoadBalancer),
			expectClusterIPs: true,
			expectNodePorts:  true,
		},
		update: svcTestCase{
			svc:         svctest.MakeService("foo", svctest.SetHeadless),
			expectError: true,
		},
	}}

	helpTestCreateUpdateDelete(t, testCases)
}

func TestFeatureExternalTrafficPolicy(t *testing.T) {
	testCases := []cudTestCase{{
		name: "ExternalName_policy:none_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeExternalName,
				svctest.SetExternalTrafficPolicy(""),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "ExternalName_policy:Cluster_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeExternalName,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster)),
			expectError: true,
		},
	}, {
		name: "ExternalName_policy:Cluster_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeExternalName,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "ExternalName_policy:Local_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeExternalName,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
			expectError: true,
		},
	}, {
		name: "ExternalName_policy:Local_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeExternalName,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "ClusterIP_policy:none_hcnp:none_policy:Cluster_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeClusterIP,
				svctest.SetExternalTrafficPolicy("")),
			expectClusterIPs: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeClusterIP,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster)),
			expectError: true,
		},
	}, {
		name: "ClusterIP_policy:none_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeClusterIP,
				svctest.SetExternalTrafficPolicy(""),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "ClusterIP_policy:Cluster_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeClusterIP,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster)),
			expectError: true,
		},
	}, {
		name: "ClusterIP_policy:Cluster_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeClusterIP,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "ClusterIP_policy:Local_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeClusterIP,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
			expectError: true,
		},
	}, {
		name: "ClusterIP_policy:Local_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeClusterIP,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "NodePort_policy:none_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeNodePort,
				svctest.SetExternalTrafficPolicy("")),
			expectError: true,
		},
	}, {
		name: "NodePort_policy:none_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeNodePort,
				svctest.SetExternalTrafficPolicy(""),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "NodePort_policy:Cluster_hcnp:none_policy:Local_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeNodePort,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: false,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeNodePort,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: false,
		},
	}, {
		name: "NodePort_policy:Cluster_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeNodePort,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "NodePort_policy:Local_hcnp:none_policy:Cluster_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeNodePort,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: false,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeNodePort,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: false,
		},
	}, {
		name: "NodePort_policy:Local_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeNodePort,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "LoadBalancer_policy:none_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy("")),
			expectError: true,
		},
	}, {
		name: "LoadBalancer_policy:none_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(""),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "LoadBalancer_policy:Cluster_hcnp:none_policy:Local_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: false,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: true,
		},
	}, {
		name: "LoadBalancer_policy:Cluster_hcnp:none_policy:Local_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: false,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(30000)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: true,
		},
	}, {
		name: "LoadBalancer_policy:Cluster_hcnp:specified",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster),
				svctest.SetHealthCheckNodePort(30000)),
			expectError: true,
		},
	}, {
		name: "LoadBalancer_policy:Local_hcnp:none_policy:Cluster_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: false,
		},
	}, {
		name: "LoadBalancer_policy:Local_hcnp:specified_policy:Cluster_hcnp:none",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(30000)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeCluster)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: false,
		},
	}, {
		name: "LoadBalancer_policy:Local_hcnp:specified_policy:Cluster_hcnp:different",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(30000)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(30001)),
			expectError: true,
		},
	}, {
		name: "LoadBalancer_policy:Local_hcnp:none_policy:Inalid",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal)),
			expectClusterIPs:          true,
			expectNodePorts:           true,
			expectHealthCheckNodePort: true,
		},
		update: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy("Invalid")),
			expectError: true,
		},
	}, {
		name: "LoadBalancer_policy:Local_hcnp:negative",
		create: svcTestCase{
			svc: svctest.MakeService("foo",
				svctest.SetTypeLoadBalancer,
				svctest.SetExternalTrafficPolicy(api.ServiceExternalTrafficPolicyTypeLocal),
				svctest.SetHealthCheckNodePort(-1)),
			expectError: true,
		},
	}}

	helpTestCreateUpdateDelete(t, testCases)
}

// FIXME: externalIPs, lbip,
// lbsourceranges, externalname, itp, PublishNotReadyAddresses,
// ipfamilypolicy and list,
// AllocateLoadBalancerNodePorts, LoadBalancerClass, status
