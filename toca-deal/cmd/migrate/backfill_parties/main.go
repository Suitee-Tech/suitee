package main

import (
	"context"
	"fmt"
	"log"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
)

func main() {
	cfg := config.LoadBaseConfig()

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer pgDB.Close()

	fmt.Println("Not implemented in Postgres pilot")
}
