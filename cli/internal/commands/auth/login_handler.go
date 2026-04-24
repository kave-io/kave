package auth

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type LoginInput struct {
}

type LoginOutput struct {
	Data any `json:"data"`
}

func RunLogin(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	email := strings.TrimSpace(os.Getenv("KAVE_EMAIL"))
	password := os.Getenv("KAVE_PASSWORD")
	if email == "" || password == "" {
		if v := strings.TrimSpace(os.Getenv("KAVE_LOGIN_EMAIL")); v != "" {
			email = v
		}
		if v := os.Getenv("KAVE_LOGIN_PASSWORD"); v != "" {
			password = v
		}
	}
	if email == "" {
		fmt.Fprint(os.Stderr, "Email: ")
		email, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		email = strings.TrimSpace(email)
	}
	if password == "" {
		fmt.Fprint(os.Stderr, "Password: ")
		password, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		password = strings.TrimSpace(password)
	}
	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}

	client := rt.Client()
	var resp struct {
		Token string `json:"token"`
		User  any    `json:"user"`
	}
	if err := client.Post(ctx, "/api/v1/auth/login", nil, map[string]any{
		"email":    email,
		"password": password,
	}, &resp); err != nil {
		return nil, err
	}
	if err := client.SaveSessionToken(resp.Token); err != nil {
		return nil, err
	}
	var user any = resp.User
	if user == nil {
		user = map[string]any{"email": email}
	}
	return &LoginOutput{Data: map[string]any{"user": user, "status": "ok"}}, nil
}
