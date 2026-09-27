package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Lookup resolves a ${NAME} placeholder. It reports false when the name is
// unknown; Parse then fails naming every unresolved placeholder at once.
type Lookup func(name string) (string, bool)

// composeSecretsDir is where Docker Compose mounts file-based secrets.
const composeSecretsDir = "/run/secrets"

// DefaultLookup resolves placeholders from, in order: a file NAME in
// $CREDENTIALS_DIRECTORY, a file /run/secrets/NAME, the environment
// variable NAME. A trailing newline of a file is dropped, since editors and
// echo leave one behind.
func DefaultLookup() Lookup {
	return lookupWith(os.Getenv("CREDENTIALS_DIRECTORY"), composeSecretsDir, os.LookupEnv)
}

// MapLookup resolves placeholders from a fixed map, for tests and tools.
func MapLookup(values map[string]string) Lookup {
	return func(name string) (string, bool) {
		v, ok := values[name]
		return v, ok
	}
}

func lookupWith(credentialsDir, secretsDir string, getenv func(string) (string, bool)) Lookup {
	return func(name string) (string, bool) {
		for _, dir := range []string{credentialsDir, secretsDir} {
			if dir == "" {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err == nil {
				return strings.TrimSuffix(string(b), "\n"), true
			}
			if !errors.Is(err, os.ErrNotExist) {
				continue
			}
		}
		return getenv(name)
	}
}
