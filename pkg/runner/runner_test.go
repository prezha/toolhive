package runner

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	rt "github.com/stacklok/toolhive/pkg/container/runtime"
	"github.com/stacklok/toolhive/pkg/logger"
)

func init() {
	logger.Initialize()
}

// TestRunner_ClientManagerConditionalCreation tests that client manager creation 
// is skipped in Kubernetes environments and attempted in local environments.
// Since we can't easily mock the client.NewManager call without significant refactoring,
// we test the conditional logic by verifying the IsKubernetesRuntime() function behaves correctly.
func TestRunner_ClientManagerConditionalCreation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		isKubernetes   bool
		expectedResult bool
	}{
		{
			name:           "should skip client manager creation in Kubernetes",
			isKubernetes:   true,
			expectedResult: true, // Should skip (return true for IsKubernetesRuntime)
		},
		{
			name:           "should create client manager outside Kubernetes",
			isKubernetes:   false,
			expectedResult: false, // Should not skip (return false for IsKubernetesRuntime)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Save original environment
			originalEnv := os.Getenv("KUBERNETES_SERVICE_HOST")
			t.Cleanup(func() {
				if originalEnv != "" {
					os.Setenv("KUBERNETES_SERVICE_HOST", originalEnv)
				} else {
					os.Unsetenv("KUBERNETES_SERVICE_HOST")
				}
			})

			// Set environment to simulate Kubernetes or local environment
			if tt.isKubernetes {
				os.Setenv("KUBERNETES_SERVICE_HOST", "test-service")
			} else {
				os.Unsetenv("KUBERNETES_SERVICE_HOST")
			}

			// Test the conditional logic that determines whether to skip client manager creation
			shouldSkipClientManager := rt.IsKubernetesRuntime()
			assert.Equal(t, tt.expectedResult, shouldSkipClientManager)

			// This test verifies that the conditional logic in runner.go:208
			// `if !rt.IsKubernetesRuntime()` will behave correctly
			if tt.isKubernetes {
				// In Kubernetes, client manager creation should be skipped
				assert.True(t, shouldSkipClientManager, "Client manager creation should be skipped in Kubernetes")
			} else {
				// Outside Kubernetes, client manager creation should be attempted
				assert.False(t, shouldSkipClientManager, "Client manager creation should be attempted outside Kubernetes")
			}
		})
	}
}

// TestIsKubernetesRuntimeConditional tests the conditional logic separately
func TestIsKubernetesRuntimeConditional(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		envValue       string
		expectedResult bool
	}{
		{
			name:           "detects Kubernetes when service host is set",
			envValue:       "kubernetes.default.svc.cluster.local",
			expectedResult: true,
		},
		{
			name:           "detects local when service host is empty",
			envValue:       "",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Save and restore environment
			originalEnv := os.Getenv("KUBERNETES_SERVICE_HOST")
			t.Cleanup(func() {
				if originalEnv != "" {
					os.Setenv("KUBERNETES_SERVICE_HOST", originalEnv)
				} else {
					os.Unsetenv("KUBERNETES_SERVICE_HOST")
				}
			})

			// Set test environment
			if tt.envValue != "" {
				os.Setenv("KUBERNETES_SERVICE_HOST", tt.envValue)
			} else {
				os.Unsetenv("KUBERNETES_SERVICE_HOST")
			}

			// Create a simple test to verify the conditional logic
			shouldSkipClientManager := rt.IsKubernetesRuntime()
			
			if tt.expectedResult {
				assert.True(t, shouldSkipClientManager, "Should skip client manager in Kubernetes")
			} else {
				assert.False(t, shouldSkipClientManager, "Should create client manager outside Kubernetes")
			}
		})
	}
}