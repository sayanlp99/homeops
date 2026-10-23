// Command devdb runs a local Postgres for development without Docker.
// Data lives in backend/.devdata; connect with
// postgres://homeops:homeops@localhost:5433/homeops?sslmode=disable
// DEVDB_PORT and DEVDB_DIR run a second, separate instance (e.g. for tests).
package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	port, dir := uint32(5433), ".devdata"
	if p, err := strconv.ParseUint(os.Getenv("DEVDB_PORT"), 10, 32); err == nil {
		port = uint32(p)
	}
	if d := os.Getenv("DEVDB_DIR"); d != "" {
		dir = d
	}
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("homeops").Password("homeops").Database("homeops").Port(port).
		DataPath(dir + "/pg").RuntimePath(dir + "/runtime").CachePath(".devdata/cache"))
	if err := pg.Start(); err != nil {
		log.Fatal(err)
	}
	log.Printf("dev postgres on :%d — Ctrl+C to stop", port)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	if err := pg.Stop(); err != nil {
		log.Fatal(err)
	}
}
