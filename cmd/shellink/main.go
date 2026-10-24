package main

import (
	"errors"
	"fmt"
	"os"

	"shellink/internal/cli"
	"shellink/internal/service"
)

func main() {
	secret, askpass := os.LookupEnv("SHELLINK_ASKPASS_SECRET")
	if askpass || len(os.Args) > 1 && os.Args[1] == "__askpass" {
		_, _ = fmt.Fprintln(os.Stdout, secret)
		return
	}
	if err := cli.Main(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var app *service.AppError
		if errors.As(err, &app) && app.Status == 400 {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
