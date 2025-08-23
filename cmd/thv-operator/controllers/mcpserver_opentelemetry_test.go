// Copyright 2024 Stacklok, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"k8s.io/utils/ptr"

	mcpv1alpha1 "github.com/stacklok/toolhive/cmd/thv-operator/api/v1alpha1"
)

func TestOpenTelemetryArgs(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, mcpv1alpha1.AddToScheme(scheme))

	tests := []struct {
		name         string
		otelConfig   *mcpv1alpha1.OpenTelemetryConfig
		expectedArgs []string
	}{
		{
			name:         "nil OpenTelemetry config",
			otelConfig:   nil,
			expectedArgs: nil,
		},
		{
			name: "basic OpenTelemetry config",
			otelConfig: &mcpv1alpha1.OpenTelemetryConfig{
				ServiceName: "test-service",
				Headers:     []string{"x-api-key=secret", "x-tenant-id=tenant1"},
				Insecure:    true,
			},
			expectedArgs: []string{
				"--otel-service-name=test-service",
				"--otel-headers=x-api-key=secret",
				"--otel-headers=x-tenant-id=tenant1",
				"--otel-insecure",
			},
		},
		{
			name: "OpenTelemetry with Prometheus metrics",
			otelConfig: &mcpv1alpha1.OpenTelemetryConfig{
				EnablePrometheusMetricsPath: true,
			},
			expectedArgs: []string{
				"--otel-enable-prometheus-metrics-path",
			},
		},
		{
			name: "complete OpenTelemetry config",
			otelConfig: &mcpv1alpha1.OpenTelemetryConfig{
				ServiceName:                 "complete-service",
				Headers:                     []string{"authorization=bearer token123"},
				Insecure:                    false,
				EnablePrometheusMetricsPath: true,
			},
			expectedArgs: []string{
				"--otel-service-name=complete-service",
				"--otel-headers=authorization=bearer token123",
				"--otel-enable-prometheus-metrics-path",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			r := &MCPServerReconciler{
				Client: client,
				Scheme: scheme,
			}

			mcpServer := &mcpv1alpha1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "default",
				},
				Spec: mcpv1alpha1.MCPServerSpec{
					Image:         "test-image:latest",
					OpenTelemetry: tt.otelConfig,
				},
			}

			args := r.generateOpenTelemetryArgs(mcpServer)
			assert.Equal(t, tt.expectedArgs, args)
		})
	}
}

func TestOpenTelemetryEnvVars(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, mcpv1alpha1.AddToScheme(scheme))

	tests := []struct {
		name        string
		otelConfig  *mcpv1alpha1.OpenTelemetryConfig
		expectedEnv []corev1.EnvVar
	}{
		{
			name:        "nil OpenTelemetry config",
			otelConfig:  nil,
			expectedEnv: nil,
		},
		{
			name: "basic OpenTelemetry config with endpoint",
			otelConfig: &mcpv1alpha1.OpenTelemetryConfig{
				Endpoint: "https://api.honeycomb.io",
			},
			expectedEnv: []corev1.EnvVar{
				{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: "https://api.honeycomb.io"},
			},
		},
		{
			name: "OpenTelemetry with sampling rate",
			otelConfig: &mcpv1alpha1.OpenTelemetryConfig{
				SamplingRate: ptr.To("0.1"),
			},
			expectedEnv: []corev1.EnvVar{
				{Name: "OTEL_TRACES_SAMPLER_ARG", Value: "0.1"},
				{Name: "OTEL_TRACES_SAMPLER", Value: "traceidratio"},
			},
		},
		{
			name: "OpenTelemetry with headers",
			otelConfig: &mcpv1alpha1.OpenTelemetryConfig{
				Headers: []string{"x-honeycomb-team=apikey123", "x-tenant=tenant1"},
			},
			expectedEnv: []corev1.EnvVar{
				{Name: "OTEL_EXPORTER_OTLP_HEADERS", Value: "x-honeycomb-team=apikey123,x-tenant=tenant1"},
			},
		},
		{
			name: "OpenTelemetry with environment variables",
			otelConfig: &mcpv1alpha1.OpenTelemetryConfig{
				EnvironmentVariables: []string{"NODE_ENV", "SERVICE_VERSION", "DEPLOYMENT_ENV"},
			},
			expectedEnv: []corev1.EnvVar{
				{Name: "TOOLHIVE_OTEL_ENV_VARS", Value: "NODE_ENV,SERVICE_VERSION,DEPLOYMENT_ENV"},
			},
		},
		{
			name: "complete OpenTelemetry config",
			otelConfig: &mcpv1alpha1.OpenTelemetryConfig{
				Endpoint:             "https://api.honeycomb.io",
				SamplingRate:         ptr.To("0.5"),
				Headers:              []string{"x-honeycomb-team=myteam"},
				EnvironmentVariables: []string{"NODE_ENV", "VERSION"},
			},
			expectedEnv: []corev1.EnvVar{
				{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: "https://api.honeycomb.io"},
				{Name: "OTEL_TRACES_SAMPLER_ARG", Value: "0.5"},
				{Name: "OTEL_TRACES_SAMPLER", Value: "traceidratio"},
				{Name: "OTEL_EXPORTER_OTLP_HEADERS", Value: "x-honeycomb-team=myteam"},
				{Name: "TOOLHIVE_OTEL_ENV_VARS", Value: "NODE_ENV,VERSION"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			r := &MCPServerReconciler{
				Client: client,
				Scheme: scheme,
			}

			mcpServer := &mcpv1alpha1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "default",
				},
				Spec: mcpv1alpha1.MCPServerSpec{
					Image:         "test-image:latest",
					OpenTelemetry: tt.otelConfig,
				},
			}

			envVars := r.generateOpenTelemetryEnvVars(mcpServer)
			assert.Equal(t, tt.expectedEnv, envVars)
		})
	}
}

func TestEqualOpenTelemetryArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		spec     *mcpv1alpha1.OpenTelemetryConfig
		args     []string
		expected bool
	}{
		{
			name:     "nil spec and no otel args",
			spec:     nil,
			args:     []string{"--transport=stdio", "--name=test"},
			expected: true,
		},
		{
			name:     "nil spec but otel args present",
			spec:     nil,
			args:     []string{"--otel-service-name=test"},
			expected: false,
		},
		{
			name: "matching service name",
			spec: &mcpv1alpha1.OpenTelemetryConfig{
				ServiceName: "test-service",
			},
			args:     []string{"--otel-service-name=test-service"},
			expected: true,
		},
		{
			name: "different service name",
			spec: &mcpv1alpha1.OpenTelemetryConfig{
				ServiceName: "test-service",
			},
			args:     []string{"--otel-service-name=other-service"},
			expected: false,
		},
		{
			name: "matching headers",
			spec: &mcpv1alpha1.OpenTelemetryConfig{
				Headers: []string{"x-api-key=secret", "x-tenant=tenant1"},
			},
			args:     []string{"--otel-headers=x-api-key=secret", "--otel-headers=x-tenant=tenant1"},
			expected: true,
		},
		{
			name: "different number of headers",
			spec: &mcpv1alpha1.OpenTelemetryConfig{
				Headers: []string{"x-api-key=secret"},
			},
			args:     []string{"--otel-headers=x-api-key=secret", "--otel-headers=x-tenant=tenant1"},
			expected: false,
		},
		{
			name: "matching insecure flag",
			spec: &mcpv1alpha1.OpenTelemetryConfig{
				Insecure: true,
			},
			args:     []string{"--otel-insecure"},
			expected: true,
		},
		{
			name: "insecure flag mismatch",
			spec: &mcpv1alpha1.OpenTelemetryConfig{
				Insecure: false,
			},
			args:     []string{"--otel-insecure"},
			expected: false,
		},
		{
			name: "matching prometheus flag",
			spec: &mcpv1alpha1.OpenTelemetryConfig{
				EnablePrometheusMetricsPath: true,
			},
			args:     []string{"--otel-enable-prometheus-metrics-path"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := equalOpenTelemetryArgs(tt.spec, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}