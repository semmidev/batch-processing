package uuid

import (
	"github.com/google/uuid"
)

// UUID is an alias for the underlying google/uuid UUID type.
// This allows drop-in compatibility with structs and methods that expect a google/uuid.UUID.
type UUID = uuid.UUID

// Nil is the nil UUID value from the google/uuid package.
var Nil = uuid.Nil

// Parse parses a string representation of a UUID.
func Parse(s string) (UUID, error) {
	return uuid.Parse(s)
}

// New generates a new UUID v7 (time-ordered UUID).
// If generation fails, it panics, mirroring the behavior of the original google/uuid.New() v4.
func New() UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic("failed to generate uuid v7: " + err.Error())
	}
	return id
}

// NewString generates a new UUID v7 and returns its string representation.
// If generation fails, it panics.
func NewString() string {
	return New().String()
}
