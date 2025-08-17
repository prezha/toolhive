package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stacklok/toolhive/pkg/environment"
	"github.com/stacklok/toolhive/pkg/logger"
)

const vaultSecretsPath = "/vault/secrets"

// processVaultSecretsDirectory detects and processes Vault Agent injected secrets
// Returns a map of environment variables to be merged with RunConfig.EnvVars
func processVaultSecretsDirectory() (map[string]string, error) {
	// Check if Vault secrets directory exists
	entries, err := os.ReadDir(vaultSecretsPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("No Vault secrets volume detected")
			return make(map[string]string), nil // Return empty map, not an error
		}
		return nil, fmt.Errorf("failed to read vault secrets directory: %w", err)
	}

	logger.Info("Vault secrets volume detected, processing injected secrets")

	allSecrets := make(map[string]string)
	processedCount := 0

	for _, entry := range entries {
		// Skip directories
		if entry.IsDir() {
			continue
		}

		// Skip hidden files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		filePath := filepath.Join(vaultSecretsPath, entry.Name())
		fileSecrets, err := processVaultSecretFile(filePath)
		if err != nil {
			logger.Warnf("Failed to process secret file %s: %v", entry.Name(), err)
			continue
		}

		// Merge secrets, with later files potentially overriding earlier ones
		for key, value := range fileSecrets {
			allSecrets[key] = value
		}
		processedCount++
	}

	logger.Infof("Processed %d Vault secret files, %d environment variables extracted", processedCount, len(allSecrets))
	return allSecrets, nil
}

// processVaultSecretFile reads and processes a single Vault secret file
// Uses existing ToolHive environment parsing utilities
func processVaultSecretFile(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Convert content to slice of KEY=VALUE lines for existing parser
	lines := strings.Split(string(content), "\n")
	var envLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Only process lines that contain '=' (KEY=VALUE format)
		if strings.Contains(line, "=") {
			envLines = append(envLines, line)
		}
	}

	if len(envLines) == 0 {
		logger.Debugf("No environment variables found in %s", filepath.Base(path))
		return make(map[string]string), nil
	}

	// Use existing ToolHive utility to parse KEY=VALUE format
	secrets, err := environment.ParseEnvironmentVariables(envLines)
	if err != nil {
		return nil, fmt.Errorf("failed to parse environment variables in %s: %w", filepath.Base(path), err)
	}

	logger.Debugf("Extracted %d environment variables from %s", len(secrets), filepath.Base(path))
	return secrets, nil
}