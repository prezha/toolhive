package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	mcpv1alpha1 "github.com/stacklok/toolhive/cmd/thv-operator/api/v1alpha1"
)

func TestMCPServerPodTemplateSpecBuilder_AllCombinations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		userTemplate           *corev1.PodTemplateSpec
		serviceAccount         *string
		secrets                []mcpv1alpha1.SecretRef
		expectedServiceAccount string
		expectedSecrets        int
		expectedContainers     int
		expectNil              bool
		description            string
	}{
		// Base cases - all nil/empty
		{
			name:        "all_nil_empty",
			expectNil:   true,
			description: "No user template, no service account, no secrets should return nil",
		},
		{
			name:         "empty_user_template_only",
			userTemplate: &corev1.PodTemplateSpec{},
			expectNil:    true,
			description:  "Empty user template with no other customizations should return nil",
		},

		// Service account only cases
		{
			name:                   "service_account_only",
			serviceAccount:         ptr.To("test-sa"),
			expectedServiceAccount: "test-sa",
			expectedContainers:     0,
			description:            "Only service account should create spec with service account",
		},
		{
			name:           "empty_service_account_only",
			serviceAccount: ptr.To(""),
			expectNil:      true,
			description:    "Empty service account string should return nil",
		},

		// Secrets only cases
		{
			name: "single_secret_only",
			secrets: []mcpv1alpha1.SecretRef{
				{Name: "secret1", Key: "key1"},
			},
			expectedSecrets:    1,
			expectedContainers: 1,
			description:        "Single secret should create MCP container with env var",
		},
		{
			name: "multiple_secrets_only",
			secrets: []mcpv1alpha1.SecretRef{
				{Name: "secret1", Key: "key1"},
				{Name: "secret2", Key: "key2", TargetEnvName: "CUSTOM_ENV"},
			},
			expectedSecrets:    2,
			expectedContainers: 1,
			description:        "Multiple secrets should create MCP container with multiple env vars",
		},
		{
			name:        "empty_secrets_only",
			secrets:     []mcpv1alpha1.SecretRef{},
			expectNil:   true,
			description: "Empty secrets slice should return nil",
		},

		// Combined service account and secrets
		{
			name:           "service_account_and_single_secret",
			serviceAccount: ptr.To("test-sa"),
			secrets: []mcpv1alpha1.SecretRef{
				{Name: "secret1", Key: "key1"},
			},
			expectedServiceAccount: "test-sa",
			expectedSecrets:        1,
			expectedContainers:     1,
			description:            "Service account and single secret should combine properly",
		},
		{
			name:           "service_account_and_multiple_secrets",
			serviceAccount: ptr.To("test-sa"),
			secrets: []mcpv1alpha1.SecretRef{
				{Name: "secret1", Key: "key1"},
				{Name: "secret2", Key: "key2", TargetEnvName: "CUSTOM_ENV"},
				{Name: "secret3", Key: "key3"},
			},
			expectedServiceAccount: "test-sa",
			expectedSecrets:        3,
			expectedContainers:     1,
			description:            "Service account and multiple secrets should combine properly",
		},

		// User template with various combinations
		{
			name: "user_template_with_existing_mcp_container_and_service_account",
			userTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "user-sa",
					Containers: []corev1.Container{
						{
							Name: "other-container",
							Env:  []corev1.EnvVar{{Name: "OTHER_ENV", Value: "value"}},
						},
						{
							Name: mcpContainerName,
							Env:  []corev1.EnvVar{{Name: "EXISTING_ENV", Value: "existing"}},
						},
					},
				},
			},
			serviceAccount: ptr.To("override-sa"),
			secrets: []mcpv1alpha1.SecretRef{
				{Name: "secret1", Key: "key1"},
			},
			expectedServiceAccount: "override-sa",
			expectedSecrets:        2, // existing + new secret env
			expectedContainers:     2,
			description:            "User template with existing MCP container should merge env vars and override service account",
		},
		{
			name: "user_template_without_mcp_container_and_secrets",
			userTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "other-container",
							Env:  []corev1.EnvVar{{Name: "OTHER_ENV", Value: "value"}},
						},
					},
				},
			},
			secrets: []mcpv1alpha1.SecretRef{
				{Name: "secret1", Key: "key1"},
			},
			expectedSecrets:    1,
			expectedContainers: 2, // other + new mcp container
			description:        "User template without MCP container should add new MCP container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Build the PodTemplateSpec
			result := NewMCPServerPodTemplateSpecBuilder(tt.userTemplate).
				WithServiceAccount(tt.serviceAccount).
				WithSecrets(tt.secrets).
				Build()

			if tt.expectNil {
				assert.Nil(t, result, "Expected nil result for case: %s", tt.description)
				return
			}

			require.NotNil(t, result, "Expected non-nil result for case: %s", tt.description)

			// Check service account
			assert.Equal(t, tt.expectedServiceAccount, result.Spec.ServiceAccountName,
				"Service account mismatch for case: %s", tt.description)

			// Check number of containers
			assert.Len(t, result.Spec.Containers, tt.expectedContainers,
				"Container count mismatch for case: %s", tt.description)

			// If we expect secrets, check the MCP container env vars
			if tt.expectedSecrets > 0 {
				mcpContainer := findMCPContainer(result.Spec.Containers)
				require.NotNil(t, mcpContainer, "Expected MCP container for case: %s", tt.description)
				assert.Len(t, mcpContainer.Env, tt.expectedSecrets,
					"Secret env var count mismatch for case: %s", tt.description)

				// Validate secret env vars structure
				for _, envVar := range mcpContainer.Env {
					if envVar.ValueFrom != nil && envVar.ValueFrom.SecretKeyRef != nil {
						assert.NotEmpty(t, envVar.Name, "Secret env var should have name")
						assert.NotEmpty(t, envVar.ValueFrom.SecretKeyRef.Name, "Secret ref should have name")
						assert.NotEmpty(t, envVar.ValueFrom.SecretKeyRef.Key, "Secret ref should have key")
					}
				}
			}
		})
	}
}

func TestMCPServerPodTemplateSpecBuilder_SecretEnvVarNaming(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		secret      mcpv1alpha1.SecretRef
		expectedEnv string
	}{
		{
			name:        "use_key_as_env_name",
			secret:      mcpv1alpha1.SecretRef{Name: "secret1", Key: "DATABASE_PASSWORD"},
			expectedEnv: "DATABASE_PASSWORD",
		},
		{
			name:        "use_custom_target_env_name",
			secret:      mcpv1alpha1.SecretRef{Name: "secret1", Key: "key1", TargetEnvName: "DB_PASSWORD"},
			expectedEnv: "DB_PASSWORD",
		},
		{
			name:        "empty_target_env_name_uses_key",
			secret:      mcpv1alpha1.SecretRef{Name: "secret1", Key: "api-token", TargetEnvName: ""},
			expectedEnv: "api-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := NewMCPServerPodTemplateSpecBuilder(nil).
				WithSecrets([]mcpv1alpha1.SecretRef{tt.secret}).
				Build()

			require.NotNil(t, result)
			mcpContainer := findMCPContainer(result.Spec.Containers)
			require.NotNil(t, mcpContainer)
			require.Len(t, mcpContainer.Env, 1)

			envVar := mcpContainer.Env[0]
			assert.Equal(t, tt.expectedEnv, envVar.Name)
			assert.Equal(t, tt.secret.Name, envVar.ValueFrom.SecretKeyRef.Name)
			assert.Equal(t, tt.secret.Key, envVar.ValueFrom.SecretKeyRef.Key)
		})
	}
}

func TestMCPServerPodTemplateSpecBuilder_WithVaultAnnotations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		vaultAgent  *mcpv1alpha1.VaultAgentConfig
		secrets     []mcpv1alpha1.SecretRef
		expected    map[string]string // expected annotations
		expectNil   bool
		description string
	}{
		{
			name:        "nil_vault_agent_returns_builder_unchanged",
			vaultAgent:  nil,
			secrets:     []mcpv1alpha1.SecretRef{},
			expectNil:   true,
			description: "Should return unchanged builder when VaultAgent is nil",
		},
		{
			name: "disabled_vault_agent_returns_builder_unchanged",
			vaultAgent: &mcpv1alpha1.VaultAgentConfig{
				Enabled: false,
				Auth: mcpv1alpha1.VaultAgentAuth{
					Role: "test-role",
				},
			},
			secrets:     []mcpv1alpha1.SecretRef{},
			expectNil:   true,
			description: "Should return unchanged builder when VaultAgent is disabled",
		},
		{
			name: "enabled_vault_agent_no_vault_secrets_returns_unchanged",
			vaultAgent: &mcpv1alpha1.VaultAgentConfig{
				Enabled: true,
				Auth: mcpv1alpha1.VaultAgentAuth{
					Role: "my-vault-role",
				},
			},
			secrets: []mcpv1alpha1.SecretRef{
				{Type: "kubernetes", Name: "k8s-secret", Key: "api-key"},
			},
			expectNil:   true,
			description: "Should return unchanged builder when no vault-type secrets are present",
		},
		{
			name: "basic_vault_agent_with_vault_secrets",
			vaultAgent: &mcpv1alpha1.VaultAgentConfig{
				Enabled: true,
				Auth: mcpv1alpha1.VaultAgentAuth{
					Role: "my-vault-role",
				},
			},
			secrets: []mcpv1alpha1.SecretRef{
				{
					Type: "vault",
					Name: "db-creds",
					Path: "secret/data/db",
				},
			},
			expected: map[string]string{
				"vault.hashicorp.com/agent-inject":                  "true",
				"vault.hashicorp.com/role":                         "my-vault-role",
				"vault.hashicorp.com/auth-path":                    "auth/kubernetes",
				"vault.hashicorp.com/agent-inject-secret-db-creds": "secret/data/db",
			},
			description: "Should generate basic Vault Agent annotations for vault-type secrets",
		},
		{
			name: "vault_agent_with_custom_auth_path",
			vaultAgent: &mcpv1alpha1.VaultAgentConfig{
				Enabled: true,
				Auth: mcpv1alpha1.VaultAgentAuth{
					Role:     "my-vault-role",
					AuthPath: "auth/custom",
				},
			},
			secrets: []mcpv1alpha1.SecretRef{
				{Type: "vault", Name: "api-key", Path: "secret/data/api"},
			},
			expected: map[string]string{
				"vault.hashicorp.com/agent-inject":                  "true",
				"vault.hashicorp.com/role":                         "my-vault-role",
				"vault.hashicorp.com/auth-path":                    "auth/custom",
				"vault.hashicorp.com/agent-inject-secret-api-key":  "secret/data/api",
			},
			description: "Should use custom auth path when provided",
		},
		{
			name: "vault_agent_with_custom_vault_address",
			vaultAgent: &mcpv1alpha1.VaultAgentConfig{
				Enabled: true,
				Auth: mcpv1alpha1.VaultAgentAuth{
					Role: "my-vault-role",
				},
				Config: &mcpv1alpha1.VaultAgentConfigSettings{
					VaultAddress: "https://vault.example.com:8200",
				},
			},
			secrets: []mcpv1alpha1.SecretRef{
				{Type: "vault", Name: "config", Path: "secret/data/config"},
			},
			expected: map[string]string{
				"vault.hashicorp.com/agent-inject":                "true",
				"vault.hashicorp.com/role":                       "my-vault-role",
				"vault.hashicorp.com/auth-path":                  "auth/kubernetes",
				"vault.hashicorp.com/service":                    "https://vault.example.com:8200",
				"vault.hashicorp.com/agent-inject-secret-config": "secret/data/config",
			},
			description: "Should include vault address when configured",
		},
		{
			name: "vault_agent_with_secrets_and_templates",
			vaultAgent: &mcpv1alpha1.VaultAgentConfig{
				Enabled: true,
				Auth: mcpv1alpha1.VaultAgentAuth{
					Role: "my-vault-role",
				},
			},
			secrets: []mcpv1alpha1.SecretRef{
				{
					Type: "vault",
					Name: "db-creds",
					Path: "secret/data/db",
					Template: `{{- with secret "secret/data/db" -}}
export DB_PASSWORD="{{ .Data.data.password }}"
{{- end -}}`,
				},
				{
					Type: "kubernetes", // Should be ignored for vault annotations
					Name: "k8s-secret",
					Key:  "api-key",
				},
				{
					Type: "vault",
					Name: "api-config",
					Path: "secret/data/api",
					Template: `{{- with secret "secret/data/api" -}}
export API_TOKEN="{{ .Data.data.token }}"
{{- end -}}`,
				},
			},
			expected: map[string]string{
				"vault.hashicorp.com/agent-inject":                      "true",
				"vault.hashicorp.com/role":                             "my-vault-role",
				"vault.hashicorp.com/auth-path":                        "auth/kubernetes",
				"vault.hashicorp.com/agent-inject-secret-db-creds":     "secret/data/db",
				"vault.hashicorp.com/agent-inject-template-db-creds": `{{- with secret "secret/data/db" -}}
export DB_PASSWORD="{{ .Data.data.password }}"
{{- end -}}`,
				"vault.hashicorp.com/agent-inject-secret-api-config":     "secret/data/api",
				"vault.hashicorp.com/agent-inject-template-api-config": `{{- with secret "secret/data/api" -}}
export API_TOKEN="{{ .Data.data.token }}"
{{- end -}}`,
			},
			description: "Should generate secret and template annotations for vault-type secrets only",
		},
		{
			name: "vault_agent_with_empty_template",
			vaultAgent: &mcpv1alpha1.VaultAgentConfig{
				Enabled: true,
				Auth: mcpv1alpha1.VaultAgentAuth{
					Role: "my-vault-role",
				},
			},
			secrets: []mcpv1alpha1.SecretRef{
				{
					Type:     "vault",
					Name:     "simple-secret",
					Path:     "secret/data/simple",
					Template: "", // Empty template should not generate template annotation
				},
			},
			expected: map[string]string{
				"vault.hashicorp.com/agent-inject":                      "true",
				"vault.hashicorp.com/role":                             "my-vault-role",
				"vault.hashicorp.com/auth-path":                        "auth/kubernetes",
				"vault.hashicorp.com/agent-inject-secret-simple-secret": "secret/data/simple",
			},
			description: "Should not generate template annotation when template is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := NewMCPServerPodTemplateSpecBuilder(nil).
				WithVaultAnnotations(tt.vaultAgent, tt.secrets).
				Build()

			if tt.expectNil {
				assert.Nil(t, result, "Expected nil result for case: %s", tt.description)
				return
			}

			require.NotNil(t, result, "Expected non-nil result for case: %s", tt.description)
			require.NotNil(t, result.ObjectMeta.Annotations, "Expected annotations for case: %s", tt.description)

			// Verify all expected annotations are present
			for key, expectedValue := range tt.expected {
				actualValue, exists := result.ObjectMeta.Annotations[key]
				assert.True(t, exists, "Expected annotation %s to exist for case: %s", key, tt.description)
				assert.Equal(t, expectedValue, actualValue, "Annotation %s value mismatch for case: %s", key, tt.description)
			}

			// Verify no unexpected template annotations for secrets without templates
			for _, secret := range tt.secrets {
				if secret.Type == "vault" && secret.Template == "" {
					templateKey := "vault.hashicorp.com/agent-inject-template-" + secret.Name
					_, exists := result.ObjectMeta.Annotations[templateKey]
					assert.False(t, exists, "Did not expect template annotation %s for secret without template", templateKey)
				}
			}
		})
	}
}

func TestMCPServerPodTemplateSpecBuilder_CombinedSecretsAndVault(t *testing.T) {
	t.Parallel()

	// Test combining Kubernetes secrets (WithSecrets) and Vault secrets (WithVaultAnnotations)
	serviceAccount := "test-sa"
	kubernetesSecrets := []mcpv1alpha1.SecretRef{
		{Type: "kubernetes", Name: "k8s-secret-1", Key: "API_KEY"},
		{Type: "kubernetes", Name: "k8s-secret-2", Key: "DB_PASSWORD", TargetEnvName: "DATABASE_PASSWORD"},
	}
	vaultAgent := &mcpv1alpha1.VaultAgentConfig{
		Enabled: true,
		Auth: mcpv1alpha1.VaultAgentAuth{
			Role: "test-role",
		},
	}
	vaultSecrets := []mcpv1alpha1.SecretRef{
		{Type: "vault", Name: "vault-config", Path: "secret/data/config"},
		{Type: "kubernetes", Name: "mixed-k8s", Key: "MIXED_KEY"}, // Should be ignored by WithVaultAnnotations
	}

	result := NewMCPServerPodTemplateSpecBuilder(nil).
		WithServiceAccount(&serviceAccount).
		WithSecrets(kubernetesSecrets).
		WithVaultAnnotations(vaultAgent, vaultSecrets).
		Build()

	require.NotNil(t, result)

	// Check service account
	assert.Equal(t, "test-sa", result.Spec.ServiceAccountName)

	// Check Kubernetes secret env vars in MCP container
	mcpContainer := findMCPContainer(result.Spec.Containers)
	require.NotNil(t, mcpContainer)
	assert.Len(t, mcpContainer.Env, 2) // 2 kubernetes secrets

	// Verify env vars
	envVarNames := make([]string, len(mcpContainer.Env))
	for i, env := range mcpContainer.Env {
		envVarNames[i] = env.Name
	}
	assert.Contains(t, envVarNames, "API_KEY")
	assert.Contains(t, envVarNames, "DATABASE_PASSWORD")

	// Check Vault Agent annotations
	annotations := result.ObjectMeta.Annotations
	require.NotNil(t, annotations)
	assert.Equal(t, "true", annotations["vault.hashicorp.com/agent-inject"])
	assert.Equal(t, "test-role", annotations["vault.hashicorp.com/role"])
	assert.Equal(t, "secret/data/config", annotations["vault.hashicorp.com/agent-inject-secret-vault-config"])
}

func TestMCPServerPodTemplateSpecBuilder_IsEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		setupBuilder   func() *MCPServerPodTemplateSpecBuilder
		expectedEmpty  bool
		expectedResult bool // true if Build() should return non-nil
	}{
		{
			name: "completely_empty",
			setupBuilder: func() *MCPServerPodTemplateSpecBuilder {
				return NewMCPServerPodTemplateSpecBuilder(nil)
			},
			expectedEmpty:  true,
			expectedResult: false,
		},
		{
			name: "with_service_account",
			setupBuilder: func() *MCPServerPodTemplateSpecBuilder {
				sa := "test-sa"
				return NewMCPServerPodTemplateSpecBuilder(nil).WithServiceAccount(&sa)
			},
			expectedEmpty:  false,
			expectedResult: true,
		},
		{
			name: "with_secrets",
			setupBuilder: func() *MCPServerPodTemplateSpecBuilder {
				return NewMCPServerPodTemplateSpecBuilder(nil).WithSecrets([]mcpv1alpha1.SecretRef{
					{Name: "secret1", Key: "key1"},
				})
			},
			expectedEmpty:  false,
			expectedResult: true,
		},
		{
			name: "with_vault_secrets_only_annotations",
			setupBuilder: func() *MCPServerPodTemplateSpecBuilder {
				vaultAgent := &mcpv1alpha1.VaultAgentConfig{
					Enabled: true,
					Auth:    mcpv1alpha1.VaultAgentAuth{Role: "test-role"},
				}
				secrets := []mcpv1alpha1.SecretRef{
					{Type: "vault", Name: "vault-secret", Path: "secret/data/test"},
				}
				return NewMCPServerPodTemplateSpecBuilder(nil).WithVaultAnnotations(vaultAgent, secrets)
			},
			expectedEmpty:  false,
			expectedResult: true,
		},
		{
			name: "with_both_secrets_and_vault",
			setupBuilder: func() *MCPServerPodTemplateSpecBuilder {
				sa := "test-sa"
				vaultAgent := &mcpv1alpha1.VaultAgentConfig{
					Enabled: true,
					Auth:    mcpv1alpha1.VaultAgentAuth{Role: "test-role"},
				}
				k8sSecrets := []mcpv1alpha1.SecretRef{
					{Type: "kubernetes", Name: "k8s-secret", Key: "key1"},
				}
				vaultSecrets := []mcpv1alpha1.SecretRef{
					{Type: "vault", Name: "vault-secret", Path: "secret/data/test"},
				}
				return NewMCPServerPodTemplateSpecBuilder(nil).
					WithServiceAccount(&sa).
					WithSecrets(k8sSecrets).
					WithVaultAnnotations(vaultAgent, vaultSecrets)
			},
			expectedEmpty:  false,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			builder := tt.setupBuilder()

			// Test isEmpty method
			isEmpty := builder.isEmpty()
			assert.Equal(t, tt.expectedEmpty, isEmpty)

			// Test that Build() respects isEmpty
			result := builder.Build()
			if tt.expectedResult {
				assert.NotNil(t, result)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

// Helper function to find MCP container in a slice
func findMCPContainer(containers []corev1.Container) *corev1.Container {
	for i, container := range containers {
		if container.Name == mcpContainerName {
			return &containers[i]
		}
	}
	return nil
}
