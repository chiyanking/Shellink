package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"shellink/internal/cli"
	"shellink/internal/service"
)

func main() {
	secret, askpass := os.LookupEnv("SHELLINK_ASKPASS_SECRET")
	if askpass || len(os.Args) > 1 && os.Args[1] == "__askpass" {
		if askpass {
			writeAskpassLog()
		}
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

func writeAskpassLog() {
	path := os.Getenv("SHELLINK_LOG")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	log.New(file, "", log.LstdFlags).Printf("[Shellink] session=%s event=ssh-askpass-invoked", os.Getenv("SHELLINK_SESSION_ID"))
}
