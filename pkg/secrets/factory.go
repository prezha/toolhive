package secrets

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"

	"github.com/adrg/xdg"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"

	"github.com/stacklok/toolhive/pkg/logger"
	"github.com/stacklok/toolhive/pkg/process"
)

const (
	// PasswordEnvVar is the environment variable used to specify the password for encrypting and decrypting secrets.
	PasswordEnvVar = "TOOLHIVE_SECRETS_PASSWORD"

	// ProviderEnvVar is the environment variable used to specify the secrets provider type.
	ProviderEnvVar = "TOOLHIVE_SECRETS_PROVIDER"

	keyringService = "toolhive"
)

// ProviderType represents an enum of the types of available secrets providers.
type ProviderType string

const (
	// EncryptedType represents the encrypted secret provider.
	EncryptedType ProviderType = "encrypted"

	// OnePasswordType represents the 1Password secret provider.
	OnePasswordType ProviderType = "1password"

	// NoneType represents the none secret provider.
	NoneType ProviderType = "none"
)

// ErrUnknownManagerType is returned when an invalid value for ProviderType is specified.
var ErrUnknownManagerType = errors.New("unknown secret manager type")

// ErrSecretsNotSetup is returned when secrets functionality is used before running setup.
var ErrSecretsNotSetup = errors.New("secrets provider not configured. " +
	"Please run 'thv secrets setup' to configure a secrets provider first")

// ErrKeyringNotAvailable is returned when the OS keyring is not available for the encrypted provider.
var ErrKeyringNotAvailable = errors.New("OS keyring is not available. " +
	"The encrypted provider requires an OS keyring to securely store passwords. " +
	"Please use a different secrets provider (e.g., 'none' for API/server environments) " +
	"or ensure your system has a keyring service available")

// IsKeyringAvailable tests if the OS keyring is available by attempting to set and delete a test value.
func IsKeyringAvailable() bool {
	testKey := "toolhive-keyring-test"
	testValue := "test"

	// Try to set a test value
	if err := keyring.Set(keyringService, testKey, testValue); err != nil {
		return false
	}

	// Clean up the test value
	_ = keyring.Delete(keyringService, testKey)

	return true
}

// CreateSecretProvider creates the specified type of secrets provider.
func CreateSecretProvider(managerType ProviderType) (Provider, error) {
	switch managerType {
	case EncryptedType:
		// Enforce keyring availability for encrypted provider
		if !IsKeyringAvailable() {
			return nil, ErrKeyringNotAvailable
		}

		password, err := GetSecretsPassword()
		if err != nil {
			return nil, fmt.Errorf("failed to get secrets password: %w", err)
		}
		// Convert to 256-bit hash for use with AES-GCM.
		key := sha256.Sum256(password)
		secretsPath, err := xdg.DataFile("toolhive/secrets_encrypted")
		if err != nil {
			return nil, fmt.Errorf("unable to access secrets file path %v", err)
		}
		return NewEncryptedManager(secretsPath, key[:])
	case OnePasswordType:
		return NewOnePasswordManager()
	case NoneType:
		return NewNoneManager()
	default:
		return nil, ErrUnknownManagerType
	}
}

// GetSecretsPassword returns the password to use for encrypting and decrypting secrets.
// It will attempt to retrieve from the OS keyring.
// If not available, it will fail with an error.
func GetSecretsPassword() ([]byte, error) {
	// Attempt to load the password from the OS keyring.
	keyringSecret, err := keyring.Get(keyringService, keyringService)
	if err == nil {
		return []byte(keyringSecret), nil
	}

	// We need to determine if the error is due to a lack of keyring on the
	// system or if the keyring is available but nothing was stored.
	if errors.Is(err, keyring.ErrNotFound) {

		// Keyring is available but no password stored - this should only happen during setup
		// We cannot ask for a password in a detached process.
		// We should never trigger this, but this ensures that if there's a bug
		// then it's easier to find.
		if process.IsDetached() {
			return nil, fmt.Errorf("detached process detected, cannot ask for password")
		}

		// Prompt for password during setup
		password, err := readPasswordStdin()
		if err != nil {
			return nil, fmt.Errorf("failed to read password: %w", err)
		}

		// Store the password in the keyring for future use
		logger.Info("writing password to os keyring")
		err = keyring.Set(keyringService, keyringService, string(password))
		if err != nil {
			return nil, fmt.Errorf("failed to store password in keyring: %w", err)
		}

		return password, nil
	}

	// Assume any other keyring error means keyring is not available
	return nil, fmt.Errorf("OS keyring is not available: %w", err)
}

func readPasswordStdin() ([]byte, error) {
	printPasswordPrompt()
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	// Start new line after receiving password to ensure errors are printed correctly.
	fmt.Println()
	if err != nil {
		return nil, fmt.Errorf("failed to read password: %w", err)
	}

	// Ensure the password is non-empty.
	if len(password) == 0 {
		return nil, errors.New("password cannot be empty")
	}
	return password, nil
}

// ResetKeyringSecret clears out the secret from the keystore (if present).
func ResetKeyringSecret() error {
	return keyring.DeleteAll(keyringService)
}

func printPasswordPrompt() {
	fmt.Print("ToolHive needs a password to secure your credentials in the OS keyring.\n" +
		"This password will be used to encrypt and decrypt API tokens and other secrets\n" +
		"that need to be accessed by MCP servers. It will be securely stored in your OS keyring\n" +
		"so you won't need to enter it each time.\n" +
		"Please enter your keyring password: ")
}
