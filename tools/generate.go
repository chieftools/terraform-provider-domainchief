// Package tools runs the code generators of the provider.
package tools

// Generate the registry documentation from the provider schema, examples and templates.
//go:generate go tool tfplugindocs generate --provider-dir .. --provider-name domainchief
