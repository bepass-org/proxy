package http

import (
	"errors"
	"net/http"
)

var (
	// ErrUnauthorized is returned when the credentials are invalid.
	ErrUnauthorized = errors.New("unauthorized")
)

// Authenticator is an interface for implementing authentication.
type Authenticator interface {
	Authenticate(username, password string) error
}

// BasicAuthenticator is a simple authenticator using a map to store credentials.
type BasicAuthenticator struct {
	credentials map[string]string
}

// NewBasicAuthenticator creates a new BasicAuthenticator.
func NewBasicAuthenticator() *BasicAuthenticator {
	return &BasicAuthenticator{
		credentials: map[string]string{
			"username": "password", // Replace with your credentials
		},
	}
}

// Authenticate checks the credentials.
func (a *BasicAuthenticator) Authenticate(username, password string) error {
	if pass, ok := a.credentials[username]; ok && pass == password {
		return nil
	}
	return ErrUnauthorized
}

// CheckAuth checks the Authorization header for Basic Authentication credentials.
func CheckAuth(authenticator Authenticator, req *http.Request) error {
	username, password, ok := req.BasicAuth()
	if !ok {
		return ErrUnauthorized
	}
	return authenticator.Authenticate(username, password)
}
