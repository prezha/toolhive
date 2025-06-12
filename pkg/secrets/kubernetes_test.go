package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func setupTestKubernetesClient(secrets ...*corev1.Secret) client.Client {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		panic(err)
	}

	objects := make([]client.Object, len(secrets))
	for i, secret := range secrets {
		objects[i] = secret
	}

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
}

func createTestSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-namespace",
		},
		Data: data,
	}
}

func TestKubernetesManager_GetSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		secretName  string
		secretData  map[string][]byte
		requestName string
		want        string
		wantErr     bool
	}{
		{
			name:        "valid secret retrieval",
			secretName:  "test-secret",
			secretData:  map[string][]byte{"key1": []byte("value1")},
			requestName: "test-secret/key1",
			want:        "value1",
			wantErr:     false,
		},
		{
			name:        "missing secret",
			secretName:  "missing-secret",
			requestName: "missing-secret/key1",
			wantErr:     true,
		},
		{
			name:        "missing key in secret",
			secretName:  "test-secret",
			secretData:  map[string][]byte{"key1": []byte("value1")},
			requestName: "test-secret/missing-key",
			wantErr:     true,
		},
		{
			name:        "invalid secret name format - no slash",
			requestName: "invalid-format",
			wantErr:     true,
		},
		{
			name:        "invalid secret name format - empty name",
			requestName: "/key1",
			wantErr:     true,
		},
		{
			name:        "invalid secret name format - empty key",
			requestName: "secret/",
			wantErr:     true,
		},
		{
			name:        "empty secret name",
			requestName: "",
			wantErr:     true,
		},
		{
			name:       "secret with multiple keys",
			secretName: "multi-secret",
			secretData: map[string][]byte{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
			},
			requestName: "multi-secret/key2",
			want:        "value2",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable for parallel execution
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var secrets []*corev1.Secret
			if tt.secretName != "" && tt.secretData != nil {
				secrets = append(secrets, createTestSecret(tt.secretName, tt.secretData))
			}

			client := setupTestKubernetesClient(secrets...)
			manager := &KubernetesManager{
				client:    client,
				namespace: "test-namespace",
			}

			got, err := manager.GetSecret(context.Background(), tt.requestName)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestKubernetesManager_SetSecret(t *testing.T) {
	t.Parallel()

	k8sClient := setupTestKubernetesClient()
	manager := &KubernetesManager{
		client:    k8sClient,
		namespace: "test-namespace",
	}

	err := manager.SetSecret(context.Background(), "test-secret", "test-value")
	assert.Error(t, err)
	assert.Equal(t, ErrKubernetesReadOnly, err)

	// Test empty name
	err = manager.SetSecret(context.Background(), "", "test-value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret name cannot be empty")
}

func TestKubernetesManager_DeleteSecret(t *testing.T) {
	t.Parallel()

	k8sClient := setupTestKubernetesClient()
	manager := &KubernetesManager{
		client:    k8sClient,
		namespace: "test-namespace",
	}

	err := manager.DeleteSecret(context.Background(), "test-secret")
	assert.Error(t, err)
	assert.Equal(t, ErrKubernetesReadOnly, err)

	// Test empty name
	err = manager.DeleteSecret(context.Background(), "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret name cannot be empty")
}

func TestKubernetesManager_ListSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		secrets []*corev1.Secret
		want    []SecretDescription
		wantErr bool
	}{
		{
			name:    "no secrets",
			secrets: []*corev1.Secret{},
			want:    []SecretDescription{},
			wantErr: false,
		},
		{
			name: "single secret with one key",
			secrets: []*corev1.Secret{
				createTestSecret("secret1", map[string][]byte{
					"key1": []byte("value1"),
				}),
			},
			want: []SecretDescription{
				{
					Key:         "secret1/key1",
					Description: "Key 'key1' from secret 'secret1'",
				},
			},
			wantErr: false,
		},
		{
			name: "single secret with multiple keys",
			secrets: []*corev1.Secret{
				createTestSecret("secret1", map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				}),
			},
			want: []SecretDescription{
				{
					Key:         "secret1/key1",
					Description: "Key 'key1' from secret 'secret1'",
				},
				{
					Key:         "secret1/key2",
					Description: "Key 'key2' from secret 'secret1'",
				},
			},
			wantErr: false,
		},
		{
			name: "multiple secrets",
			secrets: []*corev1.Secret{
				createTestSecret("secret1", map[string][]byte{
					"key1": []byte("value1"),
				}),
				createTestSecret("secret2", map[string][]byte{
					"key2": []byte("value2"),
				}),
			},
			want: []SecretDescription{
				{
					Key:         "secret1/key1",
					Description: "Key 'key1' from secret 'secret1'",
				},
				{
					Key:         "secret2/key2",
					Description: "Key 'key2' from secret 'secret2'",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable for parallel execution
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := setupTestKubernetesClient(tt.secrets...)
			manager := &KubernetesManager{
				client:    client,
				namespace: "test-namespace",
			}

			got, err := manager.ListSecrets(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestKubernetesManager_Cleanup(t *testing.T) {
	t.Parallel()

	k8sClient := setupTestKubernetesClient()
	manager := &KubernetesManager{
		client:    k8sClient,
		namespace: "test-namespace",
	}

	err := manager.Cleanup()
	assert.NoError(t, err)
}

func TestKubernetesManager_Capabilities(t *testing.T) {
	t.Parallel()

	k8sClient := setupTestKubernetesClient()
	manager := &KubernetesManager{
		client:    k8sClient,
		namespace: "test-namespace",
	}

	capabilities := manager.Capabilities()
	expected := ProviderCapabilities{
		CanRead:    true,
		CanWrite:   false,
		CanDelete:  false,
		CanList:    true,
		CanCleanup: false,
	}

	assert.Equal(t, expected, capabilities)
}

func TestCreateSecretProvider_Kubernetes(t *testing.T) {
	t.Parallel()

	// This test would require a real Kubernetes environment or more complex mocking
	// For now, we'll test that the factory recognizes the kubernetes type
	// The actual NewKubernetesManager function would need a Kubernetes cluster to test properly

	// Test that the factory includes the kubernetes type
	assert.Equal(t, ProviderType("kubernetes"), KubernetesType)
}
