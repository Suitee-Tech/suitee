package main

import (
	"context"
	"os"
	"time"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
)

func main() {
	ctx := context.Background()

	dbURI := os.Getenv("DATABASE_URI")
	if dbURI == "" {
		dbURI = "postgresql://postgres:postgres@localhost:5432/toca?sslmode=disable"
	}
	dbServiceURI := os.Getenv("DATABASE_URI_SERVICE")
	if dbServiceURI == "" {
		dbServiceURI = dbURI
	}
	rabbitURI := os.Getenv("RABBITMQ_URI")
	if rabbitURI == "" {
		rabbitURI = "amqp://toca_mq_user:change-me@localhost:5672/"
	}

	pg, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:         dbURI,
		ServiceURI:  dbServiceURI,
		Name:        "toca",
		MaxPoolSize: 5,
		MinPoolSize: 0,
	})
	if err != nil {
		panic(err)
	}
	defer pg.Close()

	bus, err := events.NewRabbitMQEventBusPG(rabbitURI, pg)
	if err != nil {
		panic(err)
	}
	defer bus.Close()

	time.Sleep(7 * time.Second)
}
