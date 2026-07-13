// Command upload-example uploads a document using airplan's Go API.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jimeh/airplan/airplan"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: upload-example DOCUMENT")
	}

	file, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{})
	if err != nil {
		return err
	}
	client, err := airplan.New(ctx, cfg)
	if err != nil {
		return err
	}

	result, err := client.Upload(ctx, airplan.Input{
		Reader: file,
		Name:   filepath.Base(file.Name()),
	})
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "airplan: warning: %s\n", warning)
	}
	fmt.Println(result.URL)
	return nil
}
