package db

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/joho/godotenv"
    "github.com/jackc/pgx/v5/pgxpool"
)

var DB *pgxpool.Pool

func Connect() {
    // Load .env
    godotenv.Load()

    dbHost := os.Getenv("DB_HOST")
    dbUser := os.Getenv("DB_USER")
    dbPass := os.Getenv("DB_PASSWORD")
    dbName := os.Getenv("DB_NAME")
    dbPort := os.Getenv("DB_PORT")

    url := fmt.Sprintf(
        "postgres://%s:%s@%s:%s/%s",
        dbUser, dbPass, dbHost, dbPort, dbName,
    )

    pool, err := pgxpool.New(context.Background(), url)
    if err != nil {
        log.Fatalf("Unable to connect to database: %v", err)
    }

    if err := pool.Ping(context.Background()); err != nil {
        log.Fatalf("Ping failed: %v", err)
    }

    log.Println("Connected to PostgreSQL")
    DB = pool
}
