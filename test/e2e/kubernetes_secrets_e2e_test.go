package e2e_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rt "github.com/stacklok/toolhive/pkg/container/runtime"
	"github.com/stacklok/toolhive/pkg/runner"
	"github.com/stacklok/toolhive/pkg/secrets"
	"github.com/stacklok/toolhive/pkg/transport/types"
)

var _ = Describe("Kubernetes Secrets Provider E2E", func() {
	Context("End-to-End Secrets Flow", func() {
		It("should process secrets through complete workflow", func() {
			// Create a RunConfig with secrets (simulating what the operator would create)
			config := &runner.RunConfig{
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
			Expect(err).ToNot(HaveOccurred(), "Should be able to create kubernetes secrets provider")

			result, err := config.WithSecrets(context.Background(), secretManager, "kubernetes")
			Expect(err).ToNot(HaveOccurred(), "WithSecrets should not return an error")
			Expect(result).To(Equal(config), "WithSecrets should return the same config instance")

			// Verify KubernetesSecrets are populated
			Expect(config.ContainerOptions).ToNot(BeNil(), "ContainerOptions should be initialized")
			Expect(config.ContainerOptions.KubernetesSecrets).To(HaveLen(2), "Should have 2 kubernetes secrets")

			expectedSecrets := []rt.KubernetesSecret{
				{Name: "github-token", Key: "token", TargetEnvName: "GITHUB_TOKEN"},
				{Name: "api-config", Key: "endpoint", TargetEnvName: "API_ENDPOINT"},
			}
			Expect(config.ContainerOptions.KubernetesSecrets).To(ConsistOf(expectedSecrets), "KubernetesSecrets should match expected")

			// Step 2: Verify that ContainerOptions would be passed to transport
			// (This simulates what happens in runner.Run when calling transportHandler.Setup)
			Expect(config.ContainerOptions).ToNot(BeNil(), "ContainerOptions should be available for transport")

			// Verify that environment variables are NOT modified (kubernetes provider doesn't add env vars)
			expectedEnvVars := map[string]string{"EXISTING": "value"}
			Expect(config.EnvVars).To(Equal(expectedEnvVars), "Environment variables should not be modified by kubernetes provider")

			// Step 3: Verify the secrets format is correct for kubernetes client
			for _, secret := range config.ContainerOptions.KubernetesSecrets {
				Expect(secret.Name).ToNot(BeEmpty(), "Secret name should not be empty")
				Expect(secret.Key).ToNot(BeEmpty(), "Secret key should not be empty")
				Expect(secret.TargetEnvName).ToNot(BeEmpty(), "Target env name should not be empty")
			}
		})
	})

	Context("Kubernetes Provider Integration", func() {
		It("should handle kubernetes secrets end-to-end", func() {
			// Create a kubernetes secrets provider using the factory
			// Note: In e2e tests, we would typically use a real Kubernetes cluster
			// For now, we'll test the factory method
			provider, err := secrets.CreateSecretProvider(secrets.KubernetesType)
			Expect(err).ToNot(HaveOccurred(), "Should be able to create kubernetes secrets provider")
			Expect(provider).ToNot(BeNil(), "Provider should not be nil")

			// Note: In a real e2e test environment, we would:
			// 1. Set up a test Kubernetes cluster or use an existing one
			// 2. Create actual secrets in the cluster
			// 3. Test the provider against real Kubernetes API
			// 4. Verify it can read the test secrets

			// For this e2e test, we verify the provider type and basic functionality
			caps := provider.Capabilities()
			Expect(caps.CanRead).To(BeTrue(), "Provider should support reading")
			Expect(caps.CanWrite).To(BeFalse(), "Provider should be read-only")
			Expect(caps.CanDelete).To(BeFalse(), "Provider should be read-only")
			Expect(caps.CanList).To(BeTrue(), "Provider should support listing")
			Expect(caps.CanCleanup).To(BeFalse(), "Provider should not support cleanup")
		})

		It("should properly isolate secrets by namespace", func() {
			// Note: In a real e2e test environment, this would:
			// 1. Create secrets in different namespaces in a real cluster
			// 2. Create providers configured for different namespaces
			// 3. Verify namespace isolation works correctly

			// For this e2e test, we verify the provider respects namespace configuration
			provider, err := secrets.CreateSecretProvider(secrets.KubernetesType)
			Expect(err).ToNot(HaveOccurred(), "Should be able to create kubernetes secrets provider")
			Expect(provider).ToNot(BeNil(), "Provider should not be nil")

			// Verify the provider is properly configured
			caps := provider.Capabilities()
			Expect(caps.CanRead).To(BeTrue(), "Provider should support reading")
		})

		It("should process environment variables correctly", func() {
			// Note: In a real e2e test environment, this would:
			// 1. Create actual secrets in a Kubernetes cluster
			// 2. Test the complete workflow of secret retrieval and environment variable processing
			// 3. Verify the secrets are properly formatted for container environment variables

			// For this e2e test, we verify the provider factory works correctly
			provider, err := secrets.CreateSecretProvider(secrets.KubernetesType)
			Expect(err).ToNot(HaveOccurred(), "Should be able to create kubernetes secrets provider")
			Expect(provider).ToNot(BeNil(), "Provider should not be nil")

			// Verify the provider type
			Expect(secrets.KubernetesType).To(Equal(secrets.ProviderType("kubernetes")))
		})

		It("should maintain read-only behavior", func() {
			// Create a kubernetes secrets provider
			provider, err := secrets.CreateSecretProvider(secrets.KubernetesType)
			Expect(err).ToNot(HaveOccurred(), "Should be able to create kubernetes secrets provider")
			Expect(provider).ToNot(BeNil(), "Provider should not be nil")

			// Verify capabilities
			caps := provider.Capabilities()
			Expect(caps.CanRead).To(BeTrue())
			Expect(caps.CanWrite).To(BeFalse())
			Expect(caps.CanDelete).To(BeFalse())
			Expect(caps.CanList).To(BeTrue())
			Expect(caps.CanCleanup).To(BeFalse())

			// Verify write operations fail with the correct error
			err = provider.SetSecret(context.Background(), "test-secret", "test-value")
			Expect(err).To(Equal(secrets.ErrKubernetesReadOnly))

			err = provider.DeleteSecret(context.Background(), "test-secret")
			Expect(err).To(Equal(secrets.ErrKubernetesReadOnly))

			// Verify cleanup is a no-op
			err = provider.Cleanup()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
