package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rt "github.com/stacklok/toolhive/pkg/container/runtime"
	"github.com/stacklok/toolhive/pkg/secrets"
)

func TestRunConfig_WithSecrets_KubernetesIntegration(t *testing.T) {
	t.Parallel()

	// Create a RunConfig with secrets
	config := &RunConfig{
		EnvVars: map[string]string{"EXISTING": "value"},
		Secrets: []string{
			"github-token/token,target=GITHUB_TOKEN",
			"api-config/endpoint,target=API_ENDPOINT",
		},
	}

	// Create a kubernetes secrets provider
	secretManager, err := secrets.CreateSecretProvider(secrets.KubernetesType)
	require.NoError(t, err, "Should be able to create kubernetes secrets provider")

	// Call WithSecrets
	result, err := config.WithSecrets(context.Background(), secretManager, "kubernetes")
	require.NoError(t, err, "WithSecrets should not return an error")
	assert.Equal(t, config, result, "WithSecrets should return the same config instance")

	// Verify that environment variables are NOT modified (kubernetes provider doesn't add env vars)
	expectedEnvVars := map[string]string{"EXISTING": "value"}
	assert.Equal(t, expectedEnvVars, config.EnvVars, "Environment variables should not be modified by kubernetes provider")

	// Verify that ContainerOptions is populated with KubernetesSecrets
	require.NotNil(t, config.ContainerOptions, "ContainerOptions should be initialized")
	require.Len(t, config.ContainerOptions.KubernetesSecrets, 2, "Should have 2 kubernetes secrets")

	// Verify the secrets are correctly parsed
	expectedSecrets := []rt.KubernetesSecret{
		{Name: "github-token", Key: "token", TargetEnvName: "GITHUB_TOKEN"},
		{Name: "api-config", Key: "endpoint", TargetEnvName: "API_ENDPOINT"},
	}
	assert.ElementsMatch(t, expectedSecrets, config.ContainerOptions.KubernetesSecrets, "KubernetesSecrets should match expected")

	// Log the secrets for debugging
	t.Logf("KubernetesSecrets populated: %+v", config.ContainerOptions.KubernetesSecrets)
}

func TestRunConfig_WithSecrets_KubernetesProvider_EmptySecrets(t *testing.T) {
	t.Parallel()

	// Create a RunConfig without secrets
	config := &RunConfig{
		EnvVars: map[string]string{"EXISTING": "value"},
		Secrets: []string{}, // No secrets
	}

	// Create a kubernetes secrets provider
	secretManager, err := secrets.CreateSecretProvider(secrets.KubernetesType)
	require.NoError(t, err, "Should be able to create kubernetes secrets provider")

	// Call WithSecrets
	result, err := config.WithSecrets(context.Background(), secretManager, "kubernetes")
	require.NoError(t, err, "WithSecrets should not return an error")
	assert.Equal(t, config, result, "WithSecrets should return the same config instance")

	// Verify that environment variables are unchanged
	expectedEnvVars := map[string]string{"EXISTING": "value"}
	assert.Equal(t, expectedEnvVars, config.EnvVars, "Environment variables should not be modified")

	// ContainerOptions should be nil or have empty KubernetesSecrets
	if config.ContainerOptions != nil {
		assert.Empty(t, config.ContainerOptions.KubernetesSecrets, "KubernetesSecrets should be empty when no secrets provided")
	}
}
