/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package http

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"k8s.io/kubernetes/pkg/probe"
	"k8s.io/kubernetes/pkg/version"
)

const FailureCode int = -1

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func TestHTTPProbeChecker(t *testing.T) {
	handleReq := func(s int, body string) func(w http.ResponseWriter) {
		return func(w http.ResponseWriter) {
			w.WriteHeader(s)
			w.Write([]byte(body))
		}
	}

	prober := New()
	testCases := []struct {
		handler func(w http.ResponseWriter)
		health  probe.Result
		// go1.5: error message changed for timeout, need to support
		// both old and new
		accBodies []string
	}{
		// The probe will be filled in below.  This is primarily testing that an HTTP GET happens.
		{
			handleReq(http.StatusOK, "ok body"),
			probe.Success,
			[]string{"ok body"},
		},
		{
			handleReq(FailureCode, "fail body"),
			probe.Failure,
			[]string{fmt.Sprintf("HTTP probe failed with statuscode: %d", FailureCode)},
		},
		{
			func(w http.ResponseWriter) {
				time.Sleep(3 * time.Second)
			},
			probe.Failure,
			[]string{
				"use of closed network connection",
				"request canceled (Client.Timeout exceeded while awaiting headers)",
			},
		},
	}
	for _, test := range testCases {
		// TODO: Close() this when fix #19254
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			test.handler(w)
		}))
		u, err := url.Parse(server.URL)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		_, port, err := net.SplitHostPort(u.Host)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		_, err = strconv.Atoi(port)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		health, output, err := prober.Probe(u, 1*time.Second)
		if test.health == probe.Unknown && err == nil {
			t.Errorf("Expected error")
		}
		if test.health != probe.Unknown && err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if health != test.health {
			t.Errorf("Expected %v, got %v", test.health, health)
		}
		if !containsAny(output, test.accBodies) {
			t.Errorf("Expected one of %#v, got %v", test.accBodies, output)
		}
	}
}

func TestHTTPProbeCheckerUserAgent(t *testing.T) {
	expected := fmt.Sprintf("Kubernetes/%s HTTP-Prober", version.Get())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.UserAgent()
		if got != expected {
			t.Errorf("User Agent mismatch, expected: %q got: %q", expected, got)
		}
	}))
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	prober := New()
	_, _, err = prober.Probe(u, 1*time.Second)
	if err != nil {
		t.Errorf("Expected error: %v", err)
	}
}
