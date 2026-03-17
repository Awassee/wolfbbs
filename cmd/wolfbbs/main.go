package main

import (
	"flag"
	"log"

	"wolfbbs/internal/app"
	"wolfbbs/internal/repository"
)

func main() {
	listenAddr := flag.String("listen", ":2222", "ssh listen address")
	dbURL := flag.String("db", "", "PostgreSQL DSN (defaults to WOLFBBS_DATABASE_URL / DATABASE_URL / PG* env)")
	flag.Parse()
	if *dbURL == "" {
		*dbURL = repository.ResolveDatabaseURL()
	}

	if err := app.Run(app.Config{ListenAddr: *listenAddr, DBURL: *dbURL}); err != nil {
		log.Fatal(err)
	}
}
