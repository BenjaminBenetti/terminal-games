// export_test.go exposes internal helpers for use in external test packages.
package registry

// Reset clears all registered games. Only intended for use in tests.
func Reset() {
	games = map[string]Game{}
}
