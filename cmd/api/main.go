package main

import (
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"

	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/index"
	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/server"
	"github.com/tqrcisio/rinha-backend-2026-detecta-fraude/internal/vectorize"
)

func main() {
	gz := env("REFS_GZ", "/home/tarcisio/rinha/rinha-de-backend-2026/resources/references.json.gz")
	bin := env("REFS_BIN", "/tmp/rinha-refs.bin")
	norm := env("NORM_JSON", "resources/normalization.json")
	mcc := env("MCC_JSON", "resources/mcc_risk.json")

	nrm, err := vectorize.Load(norm, mcc)
	if err != nil {
		log.Fatal(err)
	}

	var kd *index.KD
	if idx := os.Getenv("INDEX_BIN"); idx != "" {
		kd, err = index.Open(idx)
	} else {
		var refs *index.Refs
		refs, err = index.Load(gz, bin)
		if err == nil {
			kd = index.BuildKD(refs)
		}
	}
	if err != nil {
		log.Fatal(err)
	}

	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(150 << 20)
	kd.Warmup(20000)

	srv, err := server.New(server.NewHandler(nrm, kd))
	if err != nil {
		log.Fatal(err)
	}

	runtime.LockOSThread()

	if env("LISTEN_MODE", "tcp") == "fd" {
		lfd, err := server.SeqpacketListen(env("SOCKET_PATH", "/sockets/api.sock"))
		if err != nil {
			log.Fatal(err)
		}
		ctl, err := server.AcceptControl(lfd)
		if err != nil {
			log.Fatal(err)
		}
		log.Fatal(srv.Run(ctl, false))
	}

	port, _ := strconv.Atoi(env("PORT", "9999"))
	lfd, err := server.TCPListen(port)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening tcp :%d", port)
	log.Fatal(srv.Run(lfd, true))
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
