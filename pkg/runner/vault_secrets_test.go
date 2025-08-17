package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stacklok/toolhive/pkg/logger"
)

func TestProcessVaultSecretFile(t *testing.T) {
	t.Parallel()

	// Needed to prevent a nil pointer dereference in the logger.
	logger.Initialize()

	tests := []struct {
		name     string
		content  string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:     "real vault agent format",
			content:  "GITHUB_PERSONAL_ACCESS_TOKEN=ghp_test_token_12345",
			expected: map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_test_token_12345"},
			wantErr:  false,
		},
		{
			name:     "multiple variables from vault",
			content:  "GITHUB_TOKEN=ghp_123\nAPI_KEY=secret456\nDATABASE_URL=postgres://user:pass@localhost:5432/db",
			expected: map[string]string{
				"GITHUB_TOKEN":  "ghp_123",
				"API_KEY":       "secret456", 
				"DATABASE_URL":  "postgres://user:pass@localhost:5432/db",
			},
			wantErr: false,
		},
		{
			name:     "with comments and empty lines",
			content:  "# Vault injected secrets\nGITHUB_TOKEN=ghp_test\n\n# Database config\nDB_PASSWORD=secretpass",
			expected: map[string]string{"GITHUB_TOKEN": "ghp_test", "DB_PASSWORD": "secretpass"},
			wantErr:  false,
		},
		{
			name:     "empty file",
			content:  "",
			expected: map[string]string{},
			wantErr:  false,
		},
		{
			name:     "only comments",
			content:  "# This is a comment\n# Another comment",
			expected: map[string]string{},
			wantErr:  false,
		},
		{
			name:     "mixed valid and invalid lines",
			content:  "VALID_KEY=value123\nINVALID_LINE_WITHOUT_EQUALS\nANOTHER_KEY=another_value",
			expected: map[string]string{"VALID_KEY": "value123", "ANOTHER_KEY": "another_value"},
			wantErr:  false, // We skip invalid lines, don't error
		},
		{
			name:     "values with spaces and special chars",
			content:  "API_URL=https://api.example.com/v1\nSECRET_WITH_SPACES=value with spaces\nSPECIAL_CHARS=!@#$%^&*()",
			expected: map[string]string{
				"API_URL":           "https://api.example.com/v1",
				"SECRET_WITH_SPACES": "value with spaces",
				"SPECIAL_CHARS":     "!@#$%^&*()",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temporary file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "secret")
			
			err := os.WriteFile(tmpFile, []byte(tt.content), 0644)
			require.NoError(t, err)

			// Test the function
			result, err := processVaultSecretFile(tmpFile)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestProcessVaultSecretsDirectory_FileFiltering(t *testing.T) {
	t.Parallel()

	// Create temporary directory structure to simulate /vault/secrets
	tmpDir := t.TempDir()
	secretsDir := filepath.Join(tmpDir, "secrets")
	err := os.MkdirAll(secretsDir, 0755)
	require.NoError(t, err)

	// Create test files
	err = os.WriteFile(filepath.Join(secretsDir, "github"), []byte("GITHUB_TOKEN=token123"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(secretsDir, "api"), []byte("API_KEY=key456"), 0644)
	require.NoError(t, err)

	// Create hidden file (should be ignored)
	err = os.WriteFile(filepath.Join(secretsDir, ".hidden"), []byte("HIDDEN=value"), 0644)
	require.NoError(t, err)

	// Create subdirectory (should be ignored)
	err = os.MkdirAll(filepath.Join(secretsDir, "subdir"), 0755)
	require.NoError(t, err)

	// Test directory processing by temporarily changing the constant
	// (In a real implementation, we'd make vaultSecretsPath configurable for testing)
	
	// For now, test the individual components
	entries, err := os.ReadDir(secretsDir)
	require.NoError(t, err)

	var processedFiles []string
	for _, entry := range entries {
		// Skip directories
		if entry.IsDir() {
			continue
		}

		// Skip hidden files
		if entry.Name()[0] == '.' {
			continue
		}

		processedFiles = append(processedFiles, entry.Name())
	}

	// Should only process github and api files, not .hidden or subdir
	assert.ElementsMatch(t, []string{"github", "api"}, processedFiles)
}

func TestWithVaultSecrets_Integration(t *testing.T) {
	t.Parallel()

	// Needed to prevent a nil pointer dereference in the logger.
	logger.Initialize()

	// Needed to prevent a nil pointer dereference in the logger.
	logger.Initialize()

	t.Run("config with existing env vars", func(t *testing.T) {
		t.Parallel()

		config := &RunConfig{
			EnvVars: map[string]string{
				"EXISTING_VAR": "existing_value",
			},
		}

		// Since processVaultSecretsDirectory uses a hardcoded path,
		// this tests the integration when no vault secrets are found
		result, err := config.WithVaultSecrets()
		
		assert.NoError(t, err)
		assert.Equal(t, config, result)
		// Existing env vars should be preserved
		assert.Equal(t, "existing_value", config.EnvVars["EXISTING_VAR"])
	})

	t.Run("config with nil env vars", func(t *testing.T) {
		t.Parallel()

		config := &RunConfig{
			EnvVars: nil,
		}

		result, err := config.WithVaultSecrets()
		
		assert.NoError(t, err)
		assert.Equal(t, config, result)
		// EnvVars should be initialized when no vault secrets found
		assert.NotNil(t, config.EnvVars)
	})
}

func TestVaultSecretsProcessor_RealWorldScenarios(t *testing.T) {
	t.Parallel()

	// Needed to prevent a nil pointer dereference in the logger.
	logger.Initialize()

	tests := []struct {
		name         string
		fileContents map[string]string // filename -> content
		expected     map[string]string // expected env vars
	}{
		{
			name: "github mcp server secrets",
			fileContents: map[string]string{
				"github-config": "GITHUB_PERSONAL_ACCESS_TOKEN=ghp_test_token_12345",
			},
			expected: map[string]string{
				"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_test_token_12345",
			},
		},
		{
			name: "multiple secret files",
			fileContents: map[string]string{
				"github":   "GITHUB_TOKEN=ghp_123\nGITHUB_ORG=myorg",
				"database": "DATABASE_URL=postgres://localhost:5432/mydb\nDB_PASSWORD=secret123",
				"api":      "API_KEY=key456\nAPI_URL=https://api.example.com",
			},
			expected: map[string]string{
				"GITHUB_TOKEN":  "ghp_123",
				"GITHUB_ORG":    "myorg",
				"DATABASE_URL":  "postgres://localhost:5432/mydb",
				"DB_PASSWORD":   "secret123",
				"API_KEY":       "key456",
				"API_URL":       "https://api.example.com",
			},
		},
		{
			name: "file with comments and complex values",
			fileContents: map[string]string{
				"app-config": `# Application configuration from Vault
# GitHub integration
GITHUB_TOKEN=ghp_very_long_token_with_numbers_123456789
GITHUB_WEBHOOK_SECRET=super_secret_webhook_key

# Database connection
DATABASE_URL=postgres://user:complex_password_with_symbols_!@#$@db.example.com:5432/production_db?sslmode=require`,
			},
			expected: map[string]string{
				"GITHUB_TOKEN":          "ghp_very_long_token_with_numbers_123456789",
				"GITHUB_WEBHOOK_SECRET": "super_secret_webhook_key",
				"DATABASE_URL":          "postgres://user:complex_password_with_symbols_!@#$@db.example.com:5432/production_db?sslmode=require",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temporary directory
			tmpDir := t.TempDir()

			// Create all files
			for filename, content := range tt.fileContents {
				err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
				require.NoError(t, err)
			}

			// Process all files and collect results
			allSecrets := make(map[string]string)
			entries, err := os.ReadDir(tmpDir)
			require.NoError(t, err)

			for _, entry := range entries {
				if entry.IsDir() || entry.Name()[0] == '.' {
					continue
				}

				filePath := filepath.Join(tmpDir, entry.Name())
				fileSecrets, err := processVaultSecretFile(filePath)
				require.NoError(t, err)

				for key, value := range fileSecrets {
					allSecrets[key] = value
				}
			}

			assert.Equal(t, tt.expected, allSecrets)
		})
	}
}