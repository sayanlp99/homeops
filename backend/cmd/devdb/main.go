// Command devdb runs a local Postgres for development without Docker.
// Data lives in backend/.devdata; connect with
// postgres://homeops:homeops@localhost:5433/homeops?sslmode=disable
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("homeops").Password("homeops").Database("homeops").Port(5433).
		DataPath(".devdata/pg").RuntimePath(".devdata/runtime").CachePath(".devdata/cache"))
	if err := pg.Start(); err != nil {
		log.Fatal(err)
	}
	log.Println("dev postgres on :5433 — Ctrl+C to stop")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	if err := pg.Stop(); err != nil {
		log.Fatal(err)
	}
}
