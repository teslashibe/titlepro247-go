// Command titlepro247-login-probe smoke-tests Login + GetMe.
package main

import (
	"context"
	"fmt"
	"os"

	tp "github.com/teslashibe/titlepro247-go"
)

func main() {
	c, err := tp.New(tp.Auth{
		Username: os.Getenv("TITLEPRO247_USERNAME"),
		Password: os.Getenv("TITLEPRO247_PASSWORD"),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(1)
	}
	user, err := c.Login(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "login:", err)
		os.Exit(1)
	}
	snap := c.AuthSnapshot()
	fmt.Printf("logged in (logged_in=%v) title=%q\n", user.LoggedIn, user.DisplayName)
	cookieLen := len(snap.AuthCookie)
	fmt.Printf("auth_cookie_len=%d\n", cookieLen)
}
