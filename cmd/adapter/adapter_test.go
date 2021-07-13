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

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const certsDir = "testdata"

func TestMakeKubeconfigHTTPClient(t *testing.T) {

	tests := []struct {
		kubeconfigPath string
		inClusterAuth  bool
		success        bool
	}{
		{
			kubeconfigPath: filepath.Join(certsDir, "kubeconfig"),
			inClusterAuth:  false,
			success:        true,
		},
		{
			kubeconfigPath: filepath.Join(certsDir, "kubeconfig"),
			inClusterAuth:  true,
			success:        false,
		},
		{
			kubeconfigPath: filepath.Join(certsDir, "kubeconfig-error"),
			inClusterAuth:  false,
			success:        false,
		},
		{
			kubeconfigPath: "",
			inClusterAuth:  false,
			success:        true,
		},
	}

	os.Setenv("KUBERNETES_SERVICE_HOST", "prometheus")
	os.Setenv("KUBERNETES_SERVICE_PORT", "8080")

	for _, test := range tests {
		t.Logf("Running test for: inClusterAuth %v, kubeconfigPath %v", test.inClusterAuth, test.kubeconfigPath)
		kubeconfigHTTPClient, err := makeKubeconfigHTTPClient(test.inClusterAuth, test.kubeconfigPath)
		if test.success {
			if err != nil {
				t.Errorf("Error is %v, expected nil", err)
				continue
			}
			if kubeconfigHTTPClient.Transport == nil {
				if test.inClusterAuth || test.kubeconfigPath != "" {
					t.Error("HTTP client Transport is nil, expected http.RoundTripper")
				}
			}
		} else {
			if err == nil {
				t.Errorf("Error is nil, expected %v", err)
			}
		}
	}
}

func TestMakePrometheusCAClient(t *testing.T) {

	tests := []struct {
		caFilePath      string
		tlsCertFilePath string
		tlsKeyFilePath  string
		success         bool
		tlsUsed         bool
	}{
		{
			caFilePath:      filepath.Join(certsDir, "ca.pem"),
			tlsCertFilePath: filepath.Join(certsDir, "tlscert.crt"),
			tlsKeyFilePath:  filepath.Join(certsDir, "tlskey.key"),
			success:         true,
			tlsUsed:         true,
		},
		{
			caFilePath:      filepath.Join(certsDir, "ca-error.pem"),
			tlsCertFilePath: filepath.Join(certsDir, "tlscert.crt"),
			tlsKeyFilePath:  filepath.Join(certsDir, "tlskey.key"),
			success:         false,
			tlsUsed:         true,
		},
		{
			caFilePath:      filepath.Join(certsDir, "ca.pem"),
			tlsCertFilePath: filepath.Join(certsDir, "tlscert-error.crt"),
			tlsKeyFilePath:  filepath.Join(certsDir, "tlskey.key"),
			success:         false,
			tlsUsed:         true,
		},
		{
			caFilePath:      filepath.Join(certsDir, "ca.pem"),
			tlsCertFilePath: "",
			tlsKeyFilePath:  "",
			success:         true,
			tlsUsed:         false,
		},
	}

	for _, test := range tests {
		t.Logf("Running test for: caFilePath %v, tlsCertFilePath %v, tlsKeyFilePath %v", test.caFilePath, test.tlsCertFilePath, test.tlsKeyFilePath)
		prometheusCAClient, err := makePrometheusCAClient(test.caFilePath, test.tlsCertFilePath, test.tlsKeyFilePath)
		if test.success {
			if err != nil {
				t.Errorf("Error is %v, expected nil", err)
				continue
			}
			if prometheusCAClient.Transport.(*http.Transport).TLSClientConfig.RootCAs == nil {
				t.Error("RootCAs is nil, expected *x509.CertPool")
				continue
			}
			if test.tlsUsed {
				if prometheusCAClient.Transport.(*http.Transport).TLSClientConfig.Certificates == nil {
					t.Error("TLS certificates is nil, expected []tls.Certificate")
					continue
				}
			} else {
				if prometheusCAClient.Transport.(*http.Transport).TLSClientConfig.Certificates != nil {
					t.Errorf("TLS certificates is %+v, expected nil", prometheusCAClient.Transport.(*http.Transport).TLSClientConfig.Certificates)
				}
			}
		} else {
			if err == nil {
				t.Errorf("Error is nil, expected %v", err)
			}
		}
	}
}

func TestParseHeaderArgs(t *testing.T) {

	tests := []struct {
		args    []string
		headers http.Header
	}{
		{
			headers: http.Header{},
		},
		{
			args: []string{"foo=bar"},
			headers: http.Header{
				"Foo": []string{"bar"},
			},
		},
		{
			args: []string{"foo"},
			headers: http.Header{
				"Foo": []string{""},
			},
		},
		{
			args: []string{"foo=bar", "foo=baz", "bux=baz=23"},
			headers: http.Header{
				"Foo": []string{"bar", "baz"},
				"Bux": []string{"baz=23"},
			},
		},
	}

	for _, test := range tests {
		got := parseHeaderArgs(test.args)
		if !reflect.DeepEqual(got, test.headers) {
			t.Errorf("Expected %#v but got %#v", test.headers, got)
		}
	}
}
