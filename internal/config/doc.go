// Package config loads the file layer of the configuration: which sources and
// displays exist, their kind-specific options, and where the secrets come
// from. Domain packages never import it; the assembly decodes each section
// into the plain Config struct of the package that owns it.
//
// Secrets are never written in the file. A value may contain ${NAME}
// placeholders, resolved by a Lookup: by default from a file NAME in
// $CREDENTIALS_DIRECTORY (systemd LoadCredential=), then /run/secrets/NAME
// (Docker Compose secrets), then the environment variable NAME. Resolution
// happens on the parsed tree, scalar by scalar, so a secret with YAML
// punctuation in it cannot corrupt the document.
package config
