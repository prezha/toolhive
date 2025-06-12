package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rt "github.com/stacklok/toolhive/pkg/container/runtime"
	"github.com/stacklok/toolhive/pkg/secrets"
	"github.com/stacklok/toolhive/pkg/transport/types"
)

func TestRunConfig_EndToEndSecretsFlow(t *testing.T) {
	t.Parallel()

	// Create a RunConfig with secrets (simulating what the operator would create)
	config := &RunConfig{
		Image:         "test-image:latest",
		ContainerName: "test-container",
		Transport:     types.TransportTypeSSE,
		Port:          8080,
		TargetPort:    9000,
		Host:          "0.0.0.0",
		TargetHost:    "localhost",
		EnvVars:       map[string]string{"EXISTING": "value"},
		Secrets: []string{
			"github-token/token,target=GITHUB_TOKEN",
			"api-config/endpoint,target=API_ENDPOINT",
		},
		// Runtime will be set by the actual runtime factory in real usage
	}

	// Step 1: Process secrets with kubernetes provider (simulating what happens in runner.Run)
	secretManager, err := secrets.CreateSecretProvider(secrets.KubernetesType)
	require.NoError(t, err, "Should be able to create kubernetes secrets provider")

	result, err := config.WithSecrets(context.Background(), secretManager, "kubernetes")
	require.NoError(t, err, "WithSecrets should not return an error")
	assert.Equal(t, config, result, "WithSecrets should return the same config instance")

	// Verify KubernetesSecrets are populated
	require.NotNil(t, config.ContainerOptions, "ContainerOptions should be initialized")
	require.Len(t, config.ContainerOptions.KubernetesSecrets, 2, "Should have 2 kubernetes secrets")

	expectedSecrets := []rt.KubernetesSecret{
		{Name: "github-token", Key: "token", TargetEnvName: "GITHUB_TOKEN"},
		{Name: "api-config", Key: "endpoint", TargetEnvName: "API_ENDPOINT"},
	}
	assert.ElementsMatch(t, expectedSecrets, config.ContainerOptions.KubernetesSecrets, "KubernetesSecrets should match expected")

	// Step 2: Verify that ContainerOptions would be passed to transport
	// (This simulates what happens in runner.Run when calling transportHandler.Setup)
	assert.NotNil(t, config.ContainerOptions, "ContainerOptions should be available for transport")

	// Log the container options for debugging
	t.Logf("ContainerOptions.KubernetesSecrets: %+v", config.ContainerOptions.KubernetesSecrets)

	// Verify that environment variables are NOT modified (kubernetes provider doesn't add env vars)
	expectedEnvVars := map[string]string{"EXISTING": "value"}
	assert.Equal(t, expectedEnvVars, config.EnvVars, "Environment variables should not be modified by kubernetes provider")

	// Step 3: Verify the secrets format is correct for kubernetes client
	for _, secret := range config.ContainerOptions.KubernetesSecrets {
		assert.NotEmpty(t, secret.Name, "Secret name should not be empty")
		assert.NotEmpty(t, secret.Key, "Secret key should not be empty")
		assert.NotEmpty(t, secret.TargetEnvName, "Target env name should not be empty")
	}
}
