//go:build !js

package game

// webFlag reports a play.html?flag toggle. Only the web build has a URL to read
// flags from; natively there are none.
func webFlag(string) bool { return false }
