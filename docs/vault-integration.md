# HashiCorp Vault Integration with ToolHive

This document explains how ToolHive integrates with HashiCorp Vault Agent Injector for dynamic secret management in Kubernetes environments.

## Overview

ToolHive's Vault integration provides automatic secret injection for MCP servers without requiring manual secret management. The integration uses HashiCorp Vault Agent Injector to dynamically retrieve secrets and make them available as environment variables to MCP server containers.

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   MCPServer     │    │  Vault Agent    │    │   Vault Server  │
│   (CRD)         │───▶│   Injector      │───▶│                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ Pod Annotations │    │ Init/Sidecar    │    │ Secret Storage  │
│ (Controller)    │    │ Containers      │    │ & Policies      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │
         ▼                       ▼
┌─────────────────┐    ┌─────────────────┐
│ ProxyRunner     │    │ /vault/secrets/ │
│ Auto-Detection  │◀───│ Volume          │
└─────────────────┘    └─────────────────┘
         │
         ▼
┌─────────────────┐
│ MCP Server      │
│ Environment     │
└─────────────────┘
```

## Identity and Access Control Model

### 1. Service Account Identity

Each MCPServer automatically gets a dedicated Kubernetes service account:

```yaml
# Generated automatically by ToolHive operator
apiVersion: v1
kind: ServiceAccount
metadata:
  name: github-vault-test-proxy-runner  # Format: {mcpserver-name}-proxy-runner
  namespace: toolhive-system
```

### 2. Vault Authentication Flow

```
1. Pod starts with service account token mounted at:
   /var/run/secrets/kubernetes.io/serviceaccount/token

2. Vault Agent Injector reads pod annotations and creates init container

3. Init container authenticates to Vault using Kubernetes auth:
   - Presents service account token to Vault
   - Vault validates token with Kubernetes API
   - If valid, Vault checks if service account is authorized

4. Vault grants temporary token with associated policies

5. Vault Agent retrieves secrets using granted policies

6. Secrets are written to /vault/secrets/ volume
```

### 3. Vault Policy Structure

**Policy Definition:**
```hcl
# Policy name: toolhive-mcp-workloads
path "workload-secrets/data/*" { 
  capabilities = ["read"] 
}
```

**Policy Restrictions:**
- ✅ **Path-based**: Only `workload-secrets/data/*` accessible
- ✅ **Read-only**: No write, delete, or list capabilities
- ✅ **Time-limited**: Tokens expire (default 1h, max 4h)

### 4. Kubernetes Auth Role

**Role Configuration:**
```bash
vault write auth/kubernetes/role/toolhive-mcp-workloads \
    bound_service_account_names=github-vault-test-proxy-runner,mcp-test \
    bound_service_account_namespaces=toolhive-system \
    policies=toolhive-mcp-workloads \
    ttl=1h \
    max_ttl=4h
```

**Access Controls:**
- ✅ **Service Account Allowlist**: Only specific service accounts
- ✅ **Namespace Restriction**: Only `toolhive-system` namespace
- ✅ **Policy Binding**: Grants `toolhive-mcp-workloads` policy

## MCPServer Configuration

### Basic Vault Integration

```yaml
apiVersion: toolhive.stacklok.dev/v1alpha1
kind: MCPServer
metadata:
  name: github-vault-test
  namespace: toolhive-system
spec:
  image: ghcr.io/github/github-mcp-server:latest
  transport: stdio
  
  # Enable Vault Agent injection
  vaultAgent:
    enabled: true
    auth:
      role: "toolhive-mcp-workloads"  # Must match Vault role name
      # authPath: "auth/kubernetes"   # Optional, defaults to auth/kubernetes
    # config:                         # Optional Vault server config
    #   vaultAddress: "https://vault.example.com:8200"
  
  # Define vault-type secrets
  secrets:
  - type: "vault"                     # Secret type: vault or kubernetes
    name: "github-config"             # Unique name for this secret
    path: "workload-secrets/data/github-mcp/config"  # Vault secret path
    template: |                       # Vault template for rendering
      {{- with secret "workload-secrets/data/github-mcp/config" -}}
      GITHUB_PERSONAL_ACCESS_TOKEN={{ .Data.data.token }}
      {{- end -}}
```

### Advanced Configuration

**Security Hardening:**
```yaml
apiVersion: toolhive.stacklok.dev/v1alpha1
kind: MCPServer
metadata:
  name: github-vault-test
  namespace: toolhive-system
spec:
  # ... other config ...
  
  vaultAgent:
    enabled: true
    auth:
      role: "toolhive-mcp-workloads"
      authPath: "auth/kubernetes"  # Always specify explicitly
    config:
      vaultAddress: "https://vault.company.com:8200"
      tlsSkipVerify: false  # Never skip TLS verification in production
      tlsServerName: "vault.company.com"
      caCert: "/etc/vault-tls/ca.pem"
  
  # Production security context
  podTemplateSpec:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65534
        fsGroup: 65534
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: mcp
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          readOnlyRootFilesystem: true
```

**Vault Agent Resource Limits:**
```yaml
# Add these annotations to control Vault Agent containers
annotations:
  vault.hashicorp.com/agent-cpu-limit: "500m"
  vault.hashicorp.com/agent-memory-limit: "512Mi"
  vault.hashicorp.com/agent-cpu-request: "250m"
  vault.hashicorp.com/agent-memory-request: "256Mi"
  vault.hashicorp.com/agent-inject-file-mode-github-config: "0644"
  vault.hashicorp.com/agent-inject-file-uid-github-config: "1000"
```

### Template Syntax

Templates use Vault Agent's template syntax (based on Go templates):

```yaml
template: |
  # Single environment variable
  {{- with secret "workload-secrets/data/myapp/config" -}}
  API_KEY={{ .Data.data.api_key }}
  {{- end -}}

  # Multiple environment variables from one secret
  {{- with secret "workload-secrets/data/myapp/config" -}}
  API_KEY={{ .Data.data.api_key }}
  DATABASE_URL={{ .Data.data.database_url }}
  WEBHOOK_SECRET={{ .Data.data.webhook_secret }}
  {{- end -}}

  # Multiple secrets in one template
  {{- with secret "workload-secrets/data/github/config" -}}
  GITHUB_TOKEN={{ .Data.data.token }}
  {{- end -}}
  {{- with secret "workload-secrets/data/database/config" -}}
  DATABASE_PASSWORD={{ .Data.data.password }}
  {{- end -}}
```

**Important Notes:**
- Templates render to `KEY=value` format (not shell exports)
- Use `.Data.data.field` for KV v2 secrets
- Use `.Data.field` for KV v1 secrets
- Comments starting with `#` are ignored
- Empty lines are ignored

## Security Best Practices

### 1. Principle of Least Privilege

**Vault Policies:**
- Create separate policies for different applications
- Use specific paths, avoid wildcards when possible
- Grant only `read` capabilities unless write is required

**Kubernetes RBAC:**
- Use dedicated service accounts per MCPServer
- Limit service account permissions to minimum required
- Restrict namespace access

### 2. Secret Path Organization

```
workload-secrets/
├── data/
│   ├── github-mcp/
│   │   └── config          # GitHub MCP server secrets
│   ├── database-mcp/
│   │   └── credentials     # Database MCP server secrets
│   └── api-gateway/
│       └── tokens          # API gateway secrets
```

**Benefits:**
- Clear ownership boundaries
- Easy policy management
- Audit trail clarity

### 3. Token Lifecycle Management

**Recommended Settings:**
```bash
vault write auth/kubernetes/role/toolhive-mcp-workloads \
    ttl=1h \                # Short default lifetime
    max_ttl=4h \            # Maximum lifetime
    policies=toolhive-mcp-workloads
```

**Monitoring:**
- Monitor token usage patterns
- Set up alerts for unusual access
- Regular policy audits

### 4. Network Security

**Vault Server:**
- Use TLS for all communications
- Restrict network access to Vault
- Enable audit logging

**Kubernetes Cluster:**
- Use network policies to isolate workloads
- Secure Vault Agent Injector webhook

## Troubleshooting

### Common Issues

**1. Pod Stuck in Init**
```bash
# Check Vault Agent init container logs
kubectl logs <pod-name> -c vault-agent-init -n <namespace>

# Common causes:
# - Service account not in Vault role allowlist
# - Vault authentication path incorrect
# - Network connectivity to Vault
```

**2. Empty Secret Files**
```bash
# Check Vault Agent sidecar logs
kubectl logs <pod-name> -c vault-agent -n <namespace>

# Check secret exists in Vault
kubectl exec vault-0 -n vault -- vault kv get workload-secrets/github-mcp/config

# Common causes:
# - Secret path doesn't exist in Vault
# - Template syntax errors
# - Insufficient Vault policy permissions
```

**3. ProxyRunner Not Processing Secrets**
```bash
# Check ProxyRunner logs for auto-detection
kubectl logs <pod-name> -c toolhive -n <namespace> | grep -i vault

# Should see:
# "Vault secrets volume detected, processing injected secrets"
# "Processed X Vault secret files, Y environment variables extracted"
```

### Debugging Commands

```bash
# Check Vault Agent annotations
kubectl describe pod <pod-name> -n <namespace> | grep vault.hashicorp.com

# Verify secret file contents
kubectl exec <pod-name> -c vault-agent -n <namespace> -- cat /vault/secrets/<secret-name>

# Check environment variables in statefulset
kubectl describe statefulset <mcpserver-name> -n <namespace>

# Test Vault authentication manually
kubectl exec vault-0 -n vault -- vault auth -method=kubernetes \
    role=toolhive-mcp-workloads \
    jwt=<service-account-token>
```

## Production Deployment

### Prerequisites

**System Requirements:**
- Kubernetes 1.24+ with RBAC enabled
- HashiCorp Vault 1.14+ 
- Vault Agent Injector 1.3+
- Network connectivity between cluster and Vault server
- TLS certificates for secure communication

**Pre-flight Checks:**
```bash
# Verify cluster access
kubectl cluster-info

# Check ToolHive operator is running
kubectl get pods -n toolhive-system -l app.kubernetes.io/name=toolhive-operator

# Verify network connectivity to Vault
kubectl run test-pod --rm -i --tty --image=curlimages/curl -- \
  curl -k https://vault.company.com:8200/v1/sys/health
```

### 1. Vault Server Configuration

**High Availability with Raft Storage:**
```hcl
# vault.hcl
ui = true
cluster_addr = "https://vault:8201"
api_addr = "https://vault:8200"

# Use Raft for clustering (recommended for Kubernetes)
storage "raft" {
  path = "/vault/data"
  node_id = "vault_node_1"
  
  retry_join {
    leader_api_addr = "https://vault-0.vault-internal:8200"
  }
  retry_join {
    leader_api_addr = "https://vault-1.vault-internal:8200"
  }
}

listener "tcp" {
  address = "0.0.0.0:8200"
  tls_cert_file = "/vault/tls/server.crt"
  tls_key_file  = "/vault/tls/server.key"
  tls_min_version = "tls12"
  tls_cipher_suites = "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"
}

# Enable audit logging
audit {
  path = "audit/"
  type = "file"
  
  options {
    file_path = "/vault/logs/audit.log"
  }
}
```

**Security Hardening:**
- Enable TLS everywhere
- Use external secrets backend (Consul, etcd)
- Enable audit logging
- Regular backup strategy

### 2. Vault Agent Injector

**Production Values:**
```yaml
# vault-agent-injector values.yaml
global:
  enabled: true
  tlsDisable: false

injector:
  enabled: true
  replicas: 3
  
  # Resource limits
  resources:
    requests:
      memory: 256Mi
      cpu: 250m
    limits:
      memory: 512Mi
      cpu: 500m
      
  # Security context
  securityContext:
    runAsNonRoot: true
    runAsUser: 100
```

### 3. Multi-Environment Setup

**Development:**
```yaml
vaultAgent:
  enabled: true
  auth:
    role: "dev-mcp-workloads"
  config:
    vaultAddress: "http://vault-dev.internal:8200"
```

**Staging:**
```yaml
vaultAgent:
  enabled: true
  auth:
    role: "staging-mcp-workloads"  
  config:
    vaultAddress: "https://vault-staging.internal:8200"
```

**Production:**
```yaml
vaultAgent:
  enabled: true
  auth:
    role: "prod-mcp-workloads"
    authPath: "auth/prod-kubernetes"
  config:
    vaultAddress: "https://vault.company.com:8200"
```

### 4. Monitoring and Alerting

**Key Metrics:**
- Vault authentication success/failure rates
- Secret retrieval latency
- Token renewal patterns
- ProxyRunner secret processing time

**Recommended Alerts:**
- Vault authentication failures
- Secret file write failures
- ProxyRunner auto-detection failures
- Unusual secret access patterns

## Migration Guide

### From Static Kubernetes Secrets

1. **Audit existing secrets:**
```bash
kubectl get secrets -n toolhive-system -o yaml > existing-secrets.yaml
```

2. **Import secrets to Vault:**
```bash
# For each secret
kubectl exec vault-0 -n vault -- vault kv put \
    workload-secrets/myapp/config \
    api_key="$(kubectl get secret myapp-secret -o jsonpath='{.data.api_key}' | base64 -d)"
```

3. **Update MCPServer configurations:**
```yaml
# Before (kubernetes secret)
secrets:
- type: "kubernetes"
  name: "myapp-secret"
  key: "api_key"
  targetEnvName: "API_KEY"

# After (vault secret)
secrets:
- type: "vault"
  name: "myapp-config"
  path: "workload-secrets/data/myapp/config"
  template: |
    {{- with secret "workload-secrets/data/myapp/config" -}}
    API_KEY={{ .Data.data.api_key }}
    {{- end -}}
```

4. **Gradual rollout:**
- Test in development first
- Validate secret access and functionality
- Monitor for issues during rollout
- Keep static secrets as backup initially

## Reference

### Vault Agent Annotations

All annotations applied to pod templates:

```yaml
annotations:
  # Enable injection
  vault.hashicorp.com/agent-inject: "true"
  
  # Authentication
  vault.hashicorp.com/role: "toolhive-mcp-workloads"
  vault.hashicorp.com/auth-path: "auth/kubernetes"
  
  # Optional: Custom Vault address
  vault.hashicorp.com/service: "https://vault.example.com:8200"
  
  # Per-secret configuration  
  vault.hashicorp.com/agent-inject-secret-<name>: "<vault-path>"
  vault.hashicorp.com/agent-inject-template-<name>: |
    <template-content>
```

### Environment Variables

ProxyRunner automatically sets these from Vault secrets:
- Variables defined in templates (e.g., `GITHUB_PERSONAL_ACCESS_TOKEN`)
- Standard MCP variables (e.g., `MCP_TRANSPORT`)

### File Locations

- **Vault secrets**: `/vault/secrets/<secret-name>`
- **Service account token**: `/var/run/secrets/kubernetes.io/serviceaccount/token`
- **ProxyRunner auto-detection**: Scans `/vault/secrets/` for regular files

This integration provides secure, automated secret management for ToolHive MCP servers while maintaining strong isolation and access controls.

## Quick Start with Task Automation

ToolHive provides automated setup tasks for reproducible Vault integration:

```bash
# Create complete environment from scratch
task kind-with-vault

# Test full reproducibility (tear down and recreate)
task vault-test-reproducibility

# Verify Vault setup
task vault-test
```

**Task Dependencies:**
- `kind-with-vault` → `kind-with-toolhive-operator-latest` + `vault-install` + `vault-configure`
- `vault-configure` → Sets up authentication, policies, roles, and test secrets
- `vault-test-reproducibility` → Full end-to-end verification from clean slate