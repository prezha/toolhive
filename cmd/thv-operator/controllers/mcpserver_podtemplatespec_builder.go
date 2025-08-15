package controllers

import (
	mcpv1alpha1 "github.com/stacklok/toolhive/cmd/thv-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Vault Agent Injector annotation constants
const (
	// vaultAgentInjectAnnotation enables/disables Vault Agent injection (required: set to "true")
	vaultAgentInjectAnnotation = "vault.hashicorp.com/agent-inject"

	// vaultAgentRoleAnnotation specifies the Vault role for Kubernetes authentication (required)
	vaultAgentRoleAnnotation = "vault.hashicorp.com/role"

	// vaultAgentAuthPathAnnotation overrides the default Kubernetes auth path
	vaultAgentAuthPathAnnotation = "vault.hashicorp.com/auth-path"

	// vaultAgentServiceAnnotation overrides the default Vault server address for the agent
	vaultAgentServiceAnnotation = "vault.hashicorp.com/service"

	// vaultAgentSecretAnnotationPrefix defines secrets to inject; append unique name (e.g., "db-creds")
	vaultAgentSecretAnnotationPrefix = "vault.hashicorp.com/agent-inject-secret-"

	// vaultAgentTemplateAnnotationPrefix defines custom Consul templates; must match secret suffix
	vaultAgentTemplateAnnotationPrefix = "vault.hashicorp.com/agent-inject-template-"

	// vaultDefaultAuthPath is the standard Kubernetes auth method path in Vault
	vaultDefaultAuthPath = "auth/kubernetes"
)

// MCPServerPodTemplateSpecBuilder provides an interface for building PodTemplateSpec patches for MCP Servers
type MCPServerPodTemplateSpecBuilder struct {
	spec *corev1.PodTemplateSpec
}

// NewMCPServerPodTemplateSpecBuilder creates a new builder, optionally starting with a user-provided template
func NewMCPServerPodTemplateSpecBuilder(userTemplate *corev1.PodTemplateSpec) *MCPServerPodTemplateSpecBuilder {
	var spec *corev1.PodTemplateSpec
	if userTemplate != nil {
		spec = userTemplate.DeepCopy()
	} else {
		spec = &corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{},
			},
		}
	}

	return &MCPServerPodTemplateSpecBuilder{spec: spec}
}

// WithServiceAccount sets the service account name
func (b *MCPServerPodTemplateSpecBuilder) WithServiceAccount(serviceAccount *string) *MCPServerPodTemplateSpecBuilder {
	if serviceAccount != nil && *serviceAccount != "" {
		b.spec.Spec.ServiceAccountName = *serviceAccount
	}
	return b
}

// WithSecrets adds secret environment variables to the MCP container
func (b *MCPServerPodTemplateSpecBuilder) WithSecrets(secrets []mcpv1alpha1.SecretRef) *MCPServerPodTemplateSpecBuilder {
	if len(secrets) == 0 {
		return b
	}

	// Generate secret env vars
	secretEnvVars := make([]corev1.EnvVar, 0, len(secrets))
	for _, secret := range secrets {
		if secret.Type != mcpv1alpha1.SecretTypeKubernetes && secret.Type != "" {
			ctxLogger.Info("Skipping secret", "name", secret.Name,
				"key", secret.Key, "type", secret.Type)
			continue
		}

		if secret.Key == "" {
			ctxLogger.Info("Skipping secret with empty key", "name", secret.Name)
			continue
		}

		targetEnv := secret.Key
		if secret.TargetEnvName != "" {
			targetEnv = secret.TargetEnvName
		}

		secretEnvVars = append(secretEnvVars, corev1.EnvVar{
			Name: targetEnv,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: secret.Key,
				},
			},
		})
	}

	if len(secretEnvVars) == 0 {
		return b
	}

	// add secret env vars to MCP container
	mcpIndex := -1
	for i, container := range b.spec.Spec.Containers {
		if container.Name == mcpContainerName {
			mcpIndex = i
			break
		}
	}

	if mcpIndex >= 0 {
		// Merge env vars into existing MCP container
		b.spec.Spec.Containers[mcpIndex].Env = append(
			b.spec.Spec.Containers[mcpIndex].Env,
			secretEnvVars...,
		)
	} else {
		// Add new MCP container with env vars
		b.spec.Spec.Containers = append(b.spec.Spec.Containers, corev1.Container{
			Name: mcpContainerName,
			Env:  secretEnvVars,
		})
	}
	return b
}

// hasVaultSecrets checks if any of the secrets are of type "vault"
func hasVaultSecrets(secrets []mcpv1alpha1.SecretRef) bool {
	for _, secret := range secrets {
		if secret.Type == mcpv1alpha1.SecretTypeVault {
			return true
		}
	}
	return false
}

func (b *MCPServerPodTemplateSpecBuilder) WithVaultAnnotations(
	vaultAgent *mcpv1alpha1.VaultAgentConfig,
	secrets []mcpv1alpha1.SecretRef,
) *MCPServerPodTemplateSpecBuilder {
	if vaultAgent == nil || !vaultAgent.Enabled {
		return b
	}

	// Only generate annotations if we have vault-type secrets
	if !hasVaultSecrets(secrets) {
		return b
	}

	annotations := make(map[string]string)

	// Required Vault Agent annotations
	annotations[vaultAgentInjectAnnotation] = "true"
	annotations[vaultAgentRoleAnnotation] = vaultAgent.Auth.Role

	// Optional auth path (defaults to "auth/kubernetes" in the CRD)
	authPath := vaultAgent.Auth.AuthPath
	if authPath == "" {
		authPath = vaultDefaultAuthPath
	}
	annotations[vaultAgentAuthPathAnnotation] = authPath

	// Optional Vault address
	if vaultAgent.Config != nil && vaultAgent.Config.VaultAddress != "" {
		annotations[vaultAgentServiceAnnotation] = vaultAgent.Config.VaultAddress
	}

	// Add vault-type secrets as Vault Agent annotations
	for _, secret := range secrets {
		if secret.Type == mcpv1alpha1.SecretTypeVault {
			secretKey := vaultAgentSecretAnnotationPrefix + secret.Name
			annotations[secretKey] = secret.Path

			if secret.Template != "" {
				templateKey := vaultAgentTemplateAnnotationPrefix + secret.Name
				annotations[templateKey] = secret.Template
			}
		}
	}

	if b.spec.ObjectMeta.Annotations == nil {
		b.spec.ObjectMeta.Annotations = make(map[string]string)
	}

	for key, value := range annotations {
		b.spec.ObjectMeta.Annotations[key] = value
	}
	return b
}

// Build returns the final PodTemplateSpec, or nil if no customizations were made
func (b *MCPServerPodTemplateSpecBuilder) Build() *corev1.PodTemplateSpec {
	// Return nil if the spec is effectively empty (no meaningful customizations)
	if b.isEmpty() {
		return nil
	}
	return b.spec
}

// isEmpty checks if the builder contains any meaningful customizations
func (b *MCPServerPodTemplateSpecBuilder) isEmpty() bool {
	return b.spec.Spec.ServiceAccountName == "" &&
		len(b.spec.Spec.Containers) == 0 &&
		len(b.spec.ObjectMeta.Annotations) == 0
}
