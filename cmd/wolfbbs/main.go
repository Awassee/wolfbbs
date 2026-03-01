package main

import (
	"flag"
	"log"

	"wolfbbs/internal/app"
<<<<<<< ours
	"wolfbbs/internal/repository"
=======
>>>>>>> theirs
)

func main() {
	listenAddr := flag.String("listen", ":2222", "ssh listen address")
<<<<<<< ours
	dbURL := flag.String("db", "", "PostgreSQL DSN (defaults to WOLFBBS_DATABASE_URL / DATABASE_URL / PG* env)")
	flag.Parse()
	if *dbURL == "" {
		*dbURL = repository.ResolveDatabaseURL()
	}

	if err := app.Run(app.Config{ListenAddr: *listenAddr, DBURL: *dbURL}); err != nil {
=======
	flag.Parse()

	if err := app.Run(app.Config{ListenAddr: *listenAddr}); err != nil {
>>>>>>> theirs
		log.Fatal(err)
	}
}
