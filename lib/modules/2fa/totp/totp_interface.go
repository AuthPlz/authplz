/*
 * TOTP Module interfaces
 * This defines the interfaces required to use the TOTP module
 *
 * AuthPlz Project (https://github.com/authplz/authplz-core)
 * Copyright 2017 Ryan Kurte
 */

package totp

import (
	"time"
)

// TokenInterface Token instance interface
// Storer token objects must implement this interface
type TokenInterface interface {
	GetExtID() string
	GetName() string
	GetSecret() string
	GetCounter() uint
	SetCounter(uint)
	GetLastUsed() time.Time
	SetLastUsed(time.Time)
}

// User interface type
// Storer user objects must implement this interface
type User interface {
	GetEmail() string
}

// Storer Token store interface
// This must be implemented by a storage module to provide persistence to the module
type Storer interface {
	// Fetch a user instance by user id (should be able to remove this)
	GetUserByExtID(userid string) (interface{}, error)
	// Add a totp token to a given user
	AddTotpToken(userid, name, secret string, counter uint) (interface{}, error)
	// Fetch totp tokens for a given user
	GetTotpTokens(userid string) ([]interface{}, error)
	// Update a provided totp token
	UpdateTotpToken(token interface{}) (interface{}, error)
	// Remove a totp token
	RemoveTotpToken(token interface{}) error
}

// CompletedHandler Callback for 2fa signature completion
type CompletedHandler interface {
	SecondFactorCompleted(userid, action string)
}
