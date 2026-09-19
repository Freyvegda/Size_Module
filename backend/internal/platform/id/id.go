// Package id produces sortable-enough identifiers for API and database rows.
package id

import "github.com/google/uuid"

// New returns a new random UUID string.
func New() string { return uuid.NewString() }
