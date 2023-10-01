package socks4

// AuthenticationFunc Authentication interface is implemented
type AuthenticationFunc func(cmd Command, username string) bool

// Auth authentication processing
func (f AuthenticationFunc) Auth(cmd Command, username string) bool {
	return f(cmd, username)
}

// Authentication proxy authentication
type Authentication interface {
	Auth(cmd Command, username string) bool
}

// UserAuth basic authentication
func UserAuth(username string) Authentication {
	return AuthenticationFunc(func(cmd Command, u string) bool {
		return username == u
	})
}
