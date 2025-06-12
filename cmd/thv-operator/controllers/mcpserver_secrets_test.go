package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	mcpv1alpha1 "github.com/stacklok/toolhive/cmd/thv-operator/api/v1alpha1"
)

func TestDeploymentForMCPServerWithSecrets(t *testing.T) {
	t.Parallel()

	// Create a test MCPServer with secrets
	mcpServer := &mcpv1alpha1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-namespace",
		},
		Spec: mcpv1alpha1.MCPServerSpec{
			Image:     "test-image:latest",
			Port:      8080,
			Transport: "sse",
			Secrets: []mcpv1alpha1.SecretRef{
				{
					Name:          "github-token",
					Key:           "token",
					TargetEnvName: "GITHUB_TOKEN",
				},
				{
					Name: "api-config",
					Key:  "endpoint",
					// No TargetEnvName - should use key as default
				},
			},
		},
	}

	// Register the scheme
	s := scheme.Scheme
	s.AddKnownTypes(mcpv1alpha1.GroupVersion, &mcpv1alpha1.MCPServer{})
	s.AddKnownTypes(mcpv1alpha1.GroupVersion, &mcpv1alpha1.MCPServerList{})

	// Create the reconciler
	r := &MCPServerReconciler{
		Scheme: s,
	}

	// Generate the deployment
	deployment := r.deploymentForMCPServer(mcpServer)
	require.NotNil(t, deployment, "Deployment should not be nil")

	// Verify deployment basic properties
	assert.Equal(t, "test-server", deployment.Name)
	assert.Equal(t, "test-namespace", deployment.Namespace)

	// Get the container spec
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1, "Should have exactly one container")
	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify the container args include the secrets
	args := container.Args
	require.NotEmpty(t, args, "Container args should not be empty")

	// Check that --secret arguments are present
	secretArgs := []string{}
	for _, arg := range args {
		if len(arg) > 9 && arg[:9] == "--secret=" {
			secretArgs = append(secretArgs, arg[9:]) // Remove "--secret=" prefix
		}
	}

	// Should have 2 secret arguments
	require.Len(t, secretArgs, 2, "Should have 2 --secret arguments")

	// Verify the secrets arguments format
	expectedSecrets := []string{
		"github-token/token,target=GITHUB_TOKEN",
		"api-config/endpoint", // No target specified, should use key as default
	}

	assert.ElementsMatch(t, expectedSecrets, secretArgs, "Secret arguments should match expected format")

	// Verify that TOOLHIVE_SECRETS_PROVIDER is set to kubernetes
	envVars := container.Env
	foundSecretsProvider := false
	for _, env := range envVars {
		if env.Name == "TOOLHIVE_SECRETS_PROVIDER" {
			foundSecretsProvider = true
			assert.Equal(t, "kubernetes", env.Value, "TOOLHIVE_SECRETS_PROVIDER should be set to kubernetes")
			break
		}
	}
	assert.True(t, foundSecretsProvider, "TOOLHIVE_SECRETS_PROVIDER environment variable should be present")

	// Verify that secrets are NOT directly mounted as environment variables in the proxy container
	// (they should be handled by the kubernetes secrets provider in the MCP container)
	for _, env := range envVars {
		assert.NotEqual(t, "GITHUB_TOKEN", env.Name, "GITHUB_TOKEN should not be directly mounted in proxy container")
		assert.NotEqual(t, "endpoint", env.Name, "endpoint should not be directly mounted in proxy container")
	}
}

func TestDeploymentForMCPServerWithoutSecrets(t *testing.T) {
	t.Parallel()

	// Create a test MCPServer without secrets
	mcpServer := &mcpv1alpha1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server-no-secrets",
			Namespace: "test-namespace",
		},
		Spec: mcpv1alpha1.MCPServerSpec{
			Image:     "test-image:latest",
			Port:      8080,
			Transport: "sse",
			// No secrets specified
		},
	}

	// Register the scheme
	s := scheme.Scheme
	s.AddKnownTypes(mcpv1alpha1.GroupVersion, &mcpv1alpha1.MCPServer{})
	s.AddKnownTypes(mcpv1alpha1.GroupVersion, &mcpv1alpha1.MCPServerList{})

	// Create the reconciler
	r := &MCPServerReconciler{
		Scheme: s,
	}

	// Generate the deployment
	deployment := r.deploymentForMCPServer(mcpServer)
	require.NotNil(t, deployment, "Deployment should not be nil")

	// Get the container spec
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1, "Should have exactly one container")
	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify no --secret arguments are present
	args := container.Args
	for _, arg := range args {
		assert.False(t, len(arg) > 9 && arg[:9] == "--secret=", "Should not have any --secret arguments when no secrets are specified")
	}

	// Verify that TOOLHIVE_SECRETS_PROVIDER is still set to kubernetes
	envVars := container.Env
	foundSecretsProvider := false
	for _, env := range envVars {
		if env.Name == "TOOLHIVE_SECRETS_PROVIDER" {
			foundSecretsProvider = true
			assert.Equal(t, "kubernetes", env.Value, "TOOLHIVE_SECRETS_PROVIDER should be set to kubernetes")
			break
		}
	}
	assert.True(t, foundSecretsProvider, "TOOLHIVE_SECRETS_PROVIDER environment variable should be present")
}
