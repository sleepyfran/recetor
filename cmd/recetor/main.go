package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sleepyfran/recetor/internal/mcpserver"
	"github.com/sleepyfran/recetor/internal/recipes"
)

func main() {
	if err := run(); err != nil {
		log.Printf("recetor: %v", err)
		os.Exit(1)
	}
}

func run() error {
	databasePath := flag.String("db", "recetor.db", "path to the SQLite recipe database")
	showVersion := flag.Bool("version", false, "print the recetor version")
	flag.Parse()

	if *showVersion {
		fmt.Println(mcpserver.Version)
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	store, err := recipes.Open(ctx, *databasePath)
	if err != nil {
		return err
	}
	defer store.Close()

	server := mcpserver.New(store)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("run MCP server: %w", err)
	}
	return nil
}
