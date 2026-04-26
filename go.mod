module github.com/yourorg/testgen

go 1.25.0

require golang.org/x/tools v0.44.0

// Косвенные зависимости golang.org/x/tools.
// После git clone выполни: go mod tidy
require (
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
)
