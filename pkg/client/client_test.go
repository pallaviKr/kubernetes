/*
Copyright 2014 Google Inc. All rights reserved.

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
package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
)

// TODO: Move this to a common place, it's needed in multiple tests.
var apiPath = "/api/v1beta1"

func makeUrl(suffix string) string {
	return apiPath + suffix
}

func TestListEmptyPods(t *testing.T) {
	c := &TestClient{
		Request:  Request{Method: "GET", Path: "/pods"},
		Response: Response{StatusCode: 200, Body: api.PodList{}},
	}
	podList, err := c.Setup().ListPods(nil)
	c.Validate(t, podList, err)
}

func TestListPods(t *testing.T) {
	c := &TestClient{
		Request: Request{Method: "GET", Path: "/pods"},
		Response: Response{StatusCode: 200,
			Body: api.PodList{
				Items: []api.Pod{
					{
						CurrentState: api.PodState{
							Status: "Foobar",
						},
						Labels: map[string]string{
							"foo":  "bar",
							"name": "baz",
						},
					},
				},
			},
		},
	}
	receivedPodList, err := c.Setup().ListPods(nil)
	c.Validate(t, receivedPodList, err)
}

func TestListPodsLabels(t *testing.T) {
	c := &TestClient{
		Request: Request{Method: "GET", Path: "/pods", Query: url.Values{"labels": []string{"foo=bar,name=baz"}}},
		Response: Response{
			StatusCode: 200,
			Body: api.PodList{
				Items: []api.Pod{
					{
						CurrentState: api.PodState{
							Status: "Foobar",
						},
						Labels: map[string]string{
							"foo":  "bar",
							"name": "baz",
						},
					},
				},
			},
		},
	}
	query := map[string]string{"foo": "bar", "name": "baz"}
	receivedPodList, err := c.Setup().ListPods(query)
	c.Validate(t, receivedPodList, err)
}

func TestGetPod(t *testing.T) {
	c := &TestClient{
		Request: Request{Method: "GET", Path: "/pods/foo"},
		Response: Response{
			StatusCode: 200,
			Body: api.Pod{
				CurrentState: api.PodState{
					Status: "Foobar",
				},
				Labels: map[string]string{
					"foo":  "bar",
					"name": "baz",
				},
			},
		},
	}
	receivedPod, err := c.Setup().GetPod("foo")
	c.Validate(t, receivedPod, err)
}

func TestDeletePod(t *testing.T) {
	c := &TestClient{
		Request:  Request{Method: "DELETE", Path: "/pods/foo"},
		Response: Response{StatusCode: 200},
	}
	err := c.Setup().DeletePod("foo")
	c.Validate(t, nil, err)
}

func TestCreatePod(t *testing.T) {
	requestPod := api.Pod{
		CurrentState: api.PodState{
			Status: "Foobar",
		},
		Labels: map[string]string{
			"foo":  "bar",
			"name": "baz",
		},
	}
	c := &TestClient{
		Request: Request{Method: "POST", Path: "/pods", Body: requestPod},
		Response: Response{
			StatusCode: 200,
			Body:       requestPod,
		},
	}
	receivedPod, err := c.Setup().CreatePod(requestPod)
	c.Validate(t, receivedPod, err)
}

func TestUpdatePod(t *testing.T) {
	requestPod := api.Pod{
		JSONBase: api.JSONBase{ID: "foo"},
		CurrentState: api.PodState{
			Status: "Foobar",
		},
		Labels: map[string]string{
			"foo":  "bar",
			"name": "baz",
		},
	}
	c := &TestClient{
		Request:  Request{Method: "PUT", Path: "/pods/foo"},
		Response: Response{StatusCode: 200, Body: requestPod},
	}
	receivedPod, err := c.Setup().UpdatePod(requestPod)
	c.Validate(t, receivedPod, err)
}

func TestGetController(t *testing.T) {
	c := &TestClient{
		Request: Request{Method: "GET", Path: "/replicationControllers/foo"},
		Response: Response{
			StatusCode: 200,
			Body: api.ReplicationController{
				JSONBase: api.JSONBase{
					ID: "foo",
				},
				DesiredState: api.ReplicationControllerState{
					Replicas: 2,
				},
				Labels: map[string]string{
					"foo":  "bar",
					"name": "baz",
				},
			},
		},
	}
	receivedController, err := c.Setup().GetReplicationController("foo")
	c.Validate(t, receivedController, err)
}

func TestUpdateController(t *testing.T) {
	requestController := api.ReplicationController{
		JSONBase: api.JSONBase{
			ID: "foo",
		},
	}
	c := &TestClient{
		Request: Request{Method: "PUT", Path: "/replicationControllers/foo"},
		Response: Response{
			StatusCode: 200,
			Body: api.ReplicationController{
				JSONBase: api.JSONBase{
					ID: "foo",
				},
				DesiredState: api.ReplicationControllerState{
					Replicas: 2,
				},
				Labels: map[string]string{
					"foo":  "bar",
					"name": "baz",
				},
			},
		},
	}
	receivedController, err := c.Setup().UpdateReplicationController(requestController)
	c.Validate(t, receivedController, err)
}

func TestDeleteController(t *testing.T) {
	c := &TestClient{
		Request:  Request{Method: "DELETE", Path: "/replicationControllers/foo"},
		Response: Response{StatusCode: 200},
	}
	err := c.Setup().DeleteReplicationController("foo")
	c.Validate(t, nil, err)
}

func TestCreateController(t *testing.T) {
	requestController := api.ReplicationController{
		JSONBase: api.JSONBase{
			ID: "foo",
		},
	}
	c := &TestClient{
		Request: Request{Method: "POST", Path: "/replicationControllers", Body: requestController},
		Response: Response{
			StatusCode: 200,
			Body: api.ReplicationController{
				JSONBase: api.JSONBase{
					ID: "foo",
				},
				DesiredState: api.ReplicationControllerState{
					Replicas: 2,
				},
				Labels: map[string]string{
					"foo":  "bar",
					"name": "baz",
				},
			},
		},
	}
	receivedController, err := c.Setup().CreateReplicationController(requestController)
	c.Validate(t, receivedController, err)
}

func body(obj interface{}, raw *string) *string {
	if obj != nil {
		bs, _ := json.Marshal(obj)
		body := string(bs)
		return &body
	}
	return raw
}

type Request struct {
	Method  string
	Path    string
	Header  string
	Query   url.Values
	Body    interface{}
	RawBody *string
}

type Response struct {
	StatusCode int
	Body       interface{}
	RawBody    *string
}

type TestClient struct {
	*Client
	Request  Request
	Response Response
	Error    bool
	server   *httptest.Server
	handler  *util.FakeHandler
	Target   interface{}
}

func (c *TestClient) Setup() *TestClient {
	c.handler = &util.FakeHandler{
		StatusCode: c.Response.StatusCode,
	}
	if responseBody := body(c.Response.Body, c.Response.RawBody); responseBody != nil {
		c.handler.ResponseBody = *responseBody
	}
	c.server = httptest.NewTLSServer(c.handler)
	if c.Client == nil {
		c.Client = &Client{}
	}
	c.Client.Host = c.server.URL
	return c
}

func (c *TestClient) Validate(t *testing.T, received interface{}, err error) {
	defer c.server.Close()

	if c.Error {
		if err == nil {
			t.Errorf("error expeced for %#v, got none", c.Request)
		}
		return
	}
	if err != nil {
		t.Errorf("no error expected for %#v, got: %v", c.Request, err)
	}

	requestBody := body(c.Request.Body, c.Request.RawBody)
	c.handler.ValidateRequest(t, makeUrl(c.Request.Path), c.Request.Method, requestBody)
	if expected, received := c.Request.Query.Encode(), c.handler.RequestReceived.URL.Query().Encode(); expected != received {
		t.Errorf("bad query for request %#v: expected %s, got %s", c.Request, expected, received)
	}
	if c.Request.Header != "" {
		if c.handler.RequestReceived.Header.Get(c.Request.Header) == "" {
			t.Errorf("header %q not found in request %#v", c.Request.Header, c.handler.RequestReceived)
		}
	}

	if expected, received := requestBody, c.handler.RequestBody; expected != nil && *expected != received {
		t.Errorf("bad body for request %#v: expected %s, got %s", c.Request, expected, received)
	}

	if c.Response.Body != nil && !reflect.DeepEqual(c.Response.Body, received) {
		t.Errorf("bad response for request %#v: expeced %s, got %s", c.Request, c.Response.Body, received)
	}
}

func TestGetService(t *testing.T) {
	c := &TestClient{
		Request:  Request{Method: "GET", Path: "/services/1"},
		Response: Response{StatusCode: 200, Body: &api.Service{JSONBase: api.JSONBase{ID: "service-1"}}},
	}
	response, err := c.Setup().GetService("1")
	c.Validate(t, &response, err)
}

func TestCreateService(t *testing.T) {
	c := (&TestClient{
		Request:  Request{Method: "POST", Path: "/services", Body: &api.Service{JSONBase: api.JSONBase{ID: "service-1"}}},
		Response: Response{StatusCode: 200, Body: &api.Service{JSONBase: api.JSONBase{ID: "service-1"}}},
	}).Setup()
	response, err := c.Setup().CreateService(api.Service{JSONBase: api.JSONBase{ID: "service-1"}})
	c.Validate(t, &response, err)
}

func TestUpdateService(t *testing.T) {
	c := &TestClient{
		Request:  Request{Method: "PUT", Path: "/services/service-1", Body: &api.Service{JSONBase: api.JSONBase{ID: "service-1"}}},
		Response: Response{StatusCode: 200, Body: &api.Service{JSONBase: api.JSONBase{ID: "service-1"}}},
	}
	response, err := c.Setup().UpdateService(api.Service{JSONBase: api.JSONBase{ID: "service-1"}})
	c.Validate(t, &response, err)
}

func TestDeleteService(t *testing.T) {
	c := &TestClient{
		Request:  Request{Method: "DELETE", Path: "/services/1"},
		Response: Response{StatusCode: 200},
	}
	err := c.Setup().DeleteService("1")
	c.Validate(t, nil, err)
}

func TestMakeRequest(t *testing.T) {
	testClients := []TestClient{
		{Request: Request{Method: "GET", Path: "/good"}, Response: Response{StatusCode: 200}},
		{Request: Request{Method: "GET", Path: "/bad%ZZ"}, Error: true},
		{Client: &Client{Auth: &AuthInfo{"foo", "bar"}}, Request: Request{Method: "GET", Path: "/auth", Header: "Authorization"}, Response: Response{StatusCode: 200}},
		{Client: &Client{httpClient: http.DefaultClient}, Request: Request{Method: "GET", Path: "/nocertificate"}, Error: true},
		{Request: Request{Method: "GET", Path: "/error"}, Response: Response{StatusCode: 500}, Error: true},
		{Request: Request{Method: "POST", Path: "/faildecode"}, Response: Response{StatusCode: 200, Body: "aaaaa"}, Target: &struct{}{}, Error: true},
		{Request: Request{Method: "GET", Path: "/failread"}, Response: Response{StatusCode: 200, Body: "aaaaa"}, Target: &struct{}{}, Error: true},
	}
	for _, c := range testClients {
		response, err := c.Setup().rawRequest(c.Request.Method, c.Request.Path[1:], nil, c.Target)
		c.Validate(t, response, err)
	}
}
