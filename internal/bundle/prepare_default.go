//go:build !portable

// Package bundle prepares the native libraries an unpackaged build loads
// from beside its executable. The portable build embeds them instead; see
// prepare_portable.go.
package bundle

// Prepare is a no-op for the regular, directory-based build.
func Prepare() error { return nil }
