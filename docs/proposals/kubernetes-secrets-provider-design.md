# Kubernetes Secrets Provider Design for ToolHive MCP Servers

## Problem Statement

ToolHive MCP servers running in Kubernetes environments currently lack a native way to access Kubernetes Secret resources, forcing users to:
- Manually mount secrets as volumes in pod templates
- Use external secret management tools
- Expose secrets through environment variables in deployment manifests
- Rely on the "none" secrets provider which provides no secret management

This creates security risks, operational complexity, and breaks the principle of least privilege in Kubernetes environments.

## Goals

- Provide native Kubernetes Secret integration for MCP servers
- Maintain security through namespace isolation and RBAC
- Use existing ToolHive secrets architecture without breaking changes
- Support the standard `--secret` parameter mechanism
- Never expose secret values in command lines or logs
- Use native `valueFrom.secretKeyRef` for secure secret mounting

## Non-Goals

- Supporting secrets from other namespaces (security boundary)
- Write operations to Kubernetes secrets (read-only provider)
- Custom secret formats beyond Kubernetes native secrets
- Breaking changes to existing MCPServer CRD or CLI interface

## Architecture Overview

ToolHive uses a pluggable secrets provider system where the operator configures the provider type and the `thv` CLI processes secrets accordingly:

```
MCPServer CRD → Operator → thv CLI → Secrets Provider → Native Secret Mounting
                    ↓           ↓            ↓                    ↓
               kubernetes   --secret    Special Flow → valueFrom.secretKeyRef
```

The kubernetes provider integrates into this existing flow with a special bypass mechanism:

```
apiVersion: toolhive.stacklok.dev/v1alpha1
kind: MCPServer
spec:
  secrets:
    - name: github-token    # Kubernetes Secret name
      key: token           # Key within the secret  
      targetEnvName: TOKEN # Environment variable name
```

## Detailed Design

### 1. Kubernetes Secrets Provider (`pkg/secrets/kubernetes.go`)

The core provider implements the existing `Provider` interface with standard secret operations:

```go
type KubernetesManager struct {
    client    client.Client
    namespace string
}

func (k *KubernetesManager) GetSecret(ctx context.Context, name string) (string, error) {
    // Parse <secret-name>/<key> format
    parts := strings.SplitN(name, "/", 2)
    if len(parts) != 2 {
        return "", fmt.Errorf("invalid secret format: %s", name)
    }
    
    secretName, key := parts[0], parts[1]
    
    // Fetch from same namespace only
    secret := &corev1.Secret{}
    err := k.client.Get(ctx, types.NamespacedName{
        Name:      secretName,
        Namespace: k.namespace,
    }, secret)
    
    if err != nil {
        return "", fmt.Errorf("failed to get secret %s: %w", secretName, err)
    }
    
    value, exists := secret.Data[key]
    if !exists {
        return "", fmt.Errorf("key %s not found in secret %s", key, secretName)
    }
    
    return string(value), nil
}
```

**Provider Capabilities:**
- Read-only: `CanRead: true, CanWrite: false, CanDelete: false`
- Namespace-scoped: Only accesses secrets in the same namespace
- Format: `<secret-name>/<key>` parsing for Kubernetes secrets
- Automatic namespace detection from environment or service account

### 2. Factory Integration (`pkg/secrets/factory.go`)

Simple extension of the existing factory pattern:

```go
const (
    EncryptedType     ProviderType = "encrypted"
    OnePasswordType   ProviderType = "1password"
    NoneType          ProviderType = "none"
    KubernetesType    ProviderType = "kubernetes" // New
)

func CreateSecretProvider(managerType ProviderType) (Provider, error) {
    switch managerType {
    case KubernetesType:
        return NewKubernetesManager()
    // ... existing cases
    }
}
```

### 3. Enhanced Container Runtime Integration

**The Key Innovation: Bypass Flow for Kubernetes Secrets**

Instead of processing secrets through the normal environment variable flow, the kubernetes provider uses a special bypass mechanism:

```go
// In pkg/runner/config.go
func (c *RunConfig) WithSecrets(ctx context.Context, secretManager secrets.Provider, providerType string) (*RunConfig, error) {
    if providerType == "kubernetes" {
        // Special kubernetes flow: bypass normal processing
        return c.withKubernetesSecrets()
    }
    // Normal flow for other providers
}
```

**Container Options Extension:**
```go
// In pkg/container/runtime/types.go
type KubernetesSecret struct {
    Name          string
    Key           string
    TargetEnvName string
}

type DeployWorkloadOptions struct {
    // ... existing fields
    KubernetesSecrets []KubernetesSecret
}
```

**Secret Parsing and Container Options Population:**
```go
// In pkg/runner/config.go
func (c *RunConfig) withKubernetesSecrets() (*RunConfig, error) {
    if c.ContainerOptions == nil {
        c.ContainerOptions = rt.NewDeployWorkloadOptions()
    }

    for _, secretParam := range c.Secrets {
        // Parse "secret-name/key,target=ENV_VAR"
        parts := strings.Split(secretParam, ",target=")
        if len(parts) != 2 {
            return c, fmt.Errorf("invalid secret format: %s", secretParam)
        }

        secretRef := parts[0]
        targetEnvName := parts[1]

        // Parse "secret-name/key"
        secretParts := strings.SplitN(secretRef, "/", 2)
        if len(secretParts) != 2 {
            return c, fmt.Errorf("invalid secret reference: %s", secretRef)
        }

        // Add to KubernetesSecrets for native mounting
        c.ContainerOptions.KubernetesSecrets = append(c.ContainerOptions.KubernetesSecrets, rt.KubernetesSecret{
            Name:          secretParts[0],
            Key:           secretParts[1],
            TargetEnvName: targetEnvName,
        })
    }
    return c, nil
}
```

**Native Secret References in Kubernetes Client:**
```go
// In pkg/container/kubernetes/client.go
func addKubernetesSecrets(container *corev1apply.ContainerApplyConfiguration, secrets []runtime.KubernetesSecret) {
    for _, secret := range secrets {
        secretEnvVar := corev1apply.EnvVar().
            WithName(secret.TargetEnvName).
            WithValueFrom(corev1apply.EnvVarSource().
                WithSecretKeyRef(corev1apply.SecretKeySelector().
                    WithName(secret.Name).
                    WithKey(secret.Key)))

        container.WithEnv(secretEnvVar)
    }
}
```

This creates native Kubernetes `valueFrom.secretKeyRef` environment variables, ensuring secret values never appear in pod templates or command lines.

### 4. Operator Integration

**Provider Configuration:**
```go
// In cmd/thv-operator/controllers/mcpserver_controller.go
env = append(env, corev1.EnvVar{
    Name:  "TOOLHIVE_SECRETS_PROVIDER", 
    Value: "kubernetes",
})

env = append(env, corev1.EnvVar{
    Name:  "TOOLHIVE_NAMESPACE",
    Value: m.Namespace,
})
```

**Secret Parameter Generation:**
```go
// Generate --secret parameters for thv CLI
for _, secret := range m.Spec.Secrets {
    secretArg := fmt.Sprintf("%s/%s", secret.Name, secret.Key)
    if secret.TargetEnvName != "" {
        secretArg = fmt.Sprintf("%s,target=%s", secretArg, secret.TargetEnvName)
    }
    args = append(args, fmt.Sprintf("--secret=%s", secretArg))
}
```

**RBAC Permissions:**
```go
var defaultRBACRules = []rbacv1.PolicyRule{
    // ... existing rules
    {
        APIGroups: []string{""},
        Resources: []string{"secrets"},
        Verbs:     []string{"get", "list"}, // Read-only
    },
}
```

## Data Model

### Secret Format
```
<secret-name>/<key>,target=<env-var-name>
```

**Examples:**
- `github-token/token,target=GITHUB_TOKEN`
- `api-config/endpoint,target=API_ENDPOINT`

### MCPServer CRD Usage
```yaml
apiVersion: toolhive.stacklok.dev/v1alpha1
kind: MCPServer
metadata:
  name: github
  namespace: default
spec:
  image: ghcr.io/github/github-mcp-server
  secrets:
    - name: github-token
      key: token
      targetEnvName: GITHUB_PERSONAL_ACCESS_TOKEN
```

### Generated StatefulSet
```yaml
apiVersion: apps/v1
kind: StatefulSet
spec:
  template:
    spec:
      containers:
      - name: mcp
        env:
        - name: GITHUB_PERSONAL_ACCESS_TOKEN
          valueFrom:
            secretKeyRef:
              name: github-token
              key: token
```

## Implementation Benefits

1. **Security First**: Namespace isolation, RBAC, read-only access, no secret exposure
2. **Native Integration**: Uses Kubernetes `valueFrom.secretKeyRef` for secure secret mounting
3. **Zero Breaking Changes**: Existing MCPServer CRDs work without modification
4. **Deterministic Provider Detection**: Uses explicit provider type instead of capability inference
5. **Bypass Architecture**: Special flow prevents secret values from being processed by ToolHive
6. **Simple Integration**: Minimal changes to existing codebase

## Security Considerations

### Namespace Isolation
- Secrets only accessible within the same namespace as the MCP server
- Automatic namespace detection from service account or environment
- No cross-namespace secret access

### RBAC
- Minimal permissions: read-only access to secrets
- Scoped to specific namespace
- No write or delete capabilities

### Secret Value Protection
- Values never logged or exposed in command lines
- Native `valueFrom.secretKeyRef` prevents secret exposure in pod templates
- Bypass flow ensures secrets never processed through normal environment variable flow
- Read-only provider prevents accidental modification

### Error Handling
- Descriptive errors without exposing sensitive information
- Proper validation of secret names and keys
- Graceful handling of missing secrets or permissions

## Implementation Phases

### Phase 1: Core Provider ✅
- Created `KubernetesManager` with full `Provider` interface implementation
- Implemented secret parsing and Kubernetes client integration
- Added comprehensive unit tests

### Phase 2: Enhanced Integration ✅
- Extended container runtime with `KubernetesSecrets` field
- Modified runner configuration for kubernetes provider detection
- Updated transport layer to pass container options
- Enhanced kubernetes client for native secret mounting

### Phase 3: Operator Integration ✅
- Updated operator to use "kubernetes" provider
- Added RBAC permissions for secret access
- Fixed CLI flag parsing (`--secret` vs `--secrets`)
- Added comprehensive integration tests

### Phase 4: Production Validation ✅
- End-to-end testing in live Kubernetes cluster
- Verified native `valueFrom.secretKeyRef` generation
- Validated namespace isolation and RBAC
- Confirmed zero secret exposure in pod templates

## Success Metrics

- ✅ Zero breaking changes to existing APIs
- ✅ Complete test coverage for new provider
- ✅ Successful integration with existing secrets architecture
- ✅ Native Kubernetes secret mounting without value exposure
- ✅ Production deployment validation in kind cluster
- ✅ Deterministic provider detection mechanism

## Alternatives Considered

1. **Volume Mounting**: Rejected due to complexity and file-based secret access
2. **External Secret Operators**: Rejected to avoid additional dependencies
3. **Direct Environment Variables**: Rejected due to security concerns (secret exposure)
4. **Capability-Based Detection**: Replaced with explicit provider type for determinism
5. **Normal Secret Processing**: Rejected in favor of bypass flow for security

The implemented approach leverages existing ToolHive architecture while providing secure, native Kubernetes secret integration. The bypass flow ensures secrets are handled securely by Kubernetes itself rather than being processed by ToolHive.

## Production Readiness

The kubernetes secrets provider is production-ready with:

- **Comprehensive Testing**: Unit, integration, and end-to-end tests
- **Security Validation**: Namespace isolation, RBAC, no secret exposure
- **Live Deployment**: Successfully tested in kind cluster with real MCPServer
- **Native Integration**: Uses Kubernetes-native secret mounting mechanisms
- **Zero Regressions**: All existing functionality continues to work
- **Deterministic Operation**: Explicit provider detection and bypass flow

The implementation provides a secure, efficient way to manage secrets in Kubernetes environments while maintaining backward compatibility and following established ToolHive patterns.