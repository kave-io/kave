package paseto

import "fmt"

type ErrConfig struct{ Msg string }

func (e ErrConfig) Error() string { return "paseto config error: " + e.Msg }

type ErrInvalidToken struct{ Err error }

func (e ErrInvalidToken) Error() string { return fmt.Sprintf("invalid token: %v", e.Err) }
func (e ErrInvalidToken) Unwrap() error { return e.Err }
