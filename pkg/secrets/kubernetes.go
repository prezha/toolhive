package secrets

import (
	"context"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// ErrKubernetesReadOnly indicates that the Kubernetes secrets manager is read-only.
// It is returned by operations which attempt to change values in Kubernetes secrets.
var ErrKubernetesReadOnly = fmt.Errorf("Kubernetes secrets manager is read-only, write operations are not supported")

// KubernetesManager manages secrets in Kubernetes.
type KubernetesManager struct {
	client    client.Client
	namespace string
}

// GetSecret retrieves a secret from Kubernetes.
func (k *KubernetesManager) GetSecret(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("secret name cannot be empty")
	}

	// Parse <secret-name>/<key> format
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid secret format: %s, expected <secret-name>/<key>", name)
	}

	secretName, key := parts[0], parts[1]
	if secretName == "" || key == "" {
		return "", fmt.Errorf("invalid secret format: %s, secret name and key cannot be empty", name)
	}

	// Fetch Kubernetes Secret
	secret := &corev1.Secret{}
	err := k.client.Get(ctx, types.NamespacedName{
		Namespace: k.namespace,
		Name:      secretName,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", secretName, err)
	}

	// Extract the specific key
	value, exists := secret.Data[key]
	if !exists {
		return "", fmt.Errorf("key %s not found in secret %s", key, secretName)
	}

	return string(value), nil
}

// SetSecret is not supported for Kubernetes secrets manager.
func (*KubernetesManager) SetSecret(_ context.Context, name, _ string) error {
	if name == "" {
		return fmt.Errorf("secret name cannot be empty")
	}
	return ErrKubernetesReadOnly
}

// DeleteSecret is not supported for Kubernetes secrets manager.
func (*KubernetesManager) DeleteSecret(_ context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("secret name cannot be empty")
	}
	return ErrKubernetesReadOnly
}

// ListSecrets returns a list of available secrets in the namespace.
func (k *KubernetesManager) ListSecrets(ctx context.Context) ([]SecretDescription, error) {
	secretList := &corev1.SecretList{}
	err := k.client.List(ctx, secretList, client.InNamespace(k.namespace))
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var descriptions []SecretDescription
	for _, secret := range secretList.Items {
		for key := range secret.Data {
			descriptions = append(descriptions, SecretDescription{
				Key:         fmt.Sprintf("%s/%s", secret.Name, key),
				Description: fmt.Sprintf("Key '%s' from secret '%s'", key, secret.Name),
			})
		}
	}

	return descriptions, nil
}

// Cleanup is not needed for Kubernetes secrets manager.
func (*KubernetesManager) Cleanup() error {
	return nil
}

// Capabilities returns the capabilities of the Kubernetes provider.
// Read-only provider with listing support.
func (*KubernetesManager) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		CanRead:    true,
		CanWrite:   false,
		CanDelete:  false,
		CanList:    true,
		CanCleanup: false,
	}
}

// NewKubernetesManager creates an instance of KubernetesManager.
func NewKubernetesManager() (Provider, error) {
	// Get Kubernetes client configuration
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// Create Kubernetes client
	kubeClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Get namespace from environment variable or default to current namespace
	namespace := os.Getenv("TOOLHIVE_NAMESPACE")
	if namespace == "" {
		// Try to read from service account namespace file
		if namespaceBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			namespace = strings.TrimSpace(string(namespaceBytes))
		}
	}
	if namespace == "" {
		namespace = "default"
	}

	return &KubernetesManager{
		client:    kubeClient,
		namespace: namespace,
	}, nil
}
