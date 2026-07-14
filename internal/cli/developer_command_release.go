//go:build !devtools

package cli

func runDeveloperCommand(string, string) bool { return false }

func developerUsage(string) string { return "" }
