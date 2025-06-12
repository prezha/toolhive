package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"

	rt "github.com/stacklok/toolhive/pkg/container/runtime"
)

func TestAddKubernetesSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		secrets  []rt.KubernetesSecret
		expected []corev1.EnvVar
	}{
		{
			name:     "no secrets",
			secrets:  []rt.KubernetesSecret{},
			expected: []corev1.EnvVar{},
		},
		{
			name: "single secret",
			secrets: []rt.KubernetesSecret{
				{
					Name:          "secret1",
					Key:           "token",
					TargetEnvName: "API_TOKEN",
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: "API_TOKEN",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "secret1",
							},
							Key: "token",
						},
					},
				},
			},
		},
		{
			name: "multiple secrets",
			secrets: []rt.KubernetesSecret{
				{
					Name:          "secret1",
					Key:           "token",
					TargetEnvName: "API_TOKEN",
				},
				{
					Name:          "secret2",
					Key:           "password",
					TargetEnvName: "DB_PASSWORD",
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: "API_TOKEN",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "secret1",
							},
							Key: "token",
						},
					},
				},
				{
					Name: "DB_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "secret2",
							},
							Key: "password",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a container apply configuration
			container := corev1apply.Container()

			// Call the function under test
			addKubernetesSecrets(container, tt.secrets)

			// Extract the environment variables from the container
			var actualEnvVars []corev1.EnvVar
			if container.Env != nil {
				for _, envApply := range container.Env {
					envVar := corev1.EnvVar{}
					if envApply.Name != nil {
						envVar.Name = *envApply.Name
					}
					if envApply.Value != nil {
						envVar.Value = *envApply.Value
					}
					if envApply.ValueFrom != nil {
						envVar.ValueFrom = &corev1.EnvVarSource{}
						if envApply.ValueFrom.SecretKeyRef != nil {
							envVar.ValueFrom.SecretKeyRef = &corev1.SecretKeySelector{}
							if envApply.ValueFrom.SecretKeyRef.Name != nil {
								envVar.ValueFrom.SecretKeyRef.Name = *envApply.ValueFrom.SecretKeyRef.Name
							}
							if envApply.ValueFrom.SecretKeyRef.Key != nil {
								envVar.ValueFrom.SecretKeyRef.Key = *envApply.ValueFrom.SecretKeyRef.Key
							}
						}
					}
					actualEnvVars = append(actualEnvVars, envVar)
				}
			}

			// Verify the results
			require.Len(t, actualEnvVars, len(tt.expected), "Number of environment variables should match")

			for i, expected := range tt.expected {
				actual := actualEnvVars[i]
				assert.Equal(t, expected.Name, actual.Name, "Environment variable name should match")
				assert.Equal(t, expected.Value, actual.Value, "Environment variable value should match")

				if expected.ValueFrom != nil {
					require.NotNil(t, actual.ValueFrom, "ValueFrom should not be nil")
					if expected.ValueFrom.SecretKeyRef != nil {
						require.NotNil(t, actual.ValueFrom.SecretKeyRef, "SecretKeyRef should not be nil")
						assert.Equal(t, expected.ValueFrom.SecretKeyRef.Name, actual.ValueFrom.SecretKeyRef.Name, "Secret name should match")
						assert.Equal(t, expected.ValueFrom.SecretKeyRef.Key, actual.ValueFrom.SecretKeyRef.Key, "Secret key should match")
					}
				} else {
					assert.Nil(t, actual.ValueFrom, "ValueFrom should be nil")
				}
			}
		})
	}
}
