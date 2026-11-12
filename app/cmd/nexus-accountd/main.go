// nexus-accountd runs the NEXUS account service (package accountd).
//
//	nexus-accountd -data /var/lib/nexus-accountd -listen 127.0.0.1:8480 -trust-proxy
//
// Run it behind a TLS terminator (see accountd/deploy), or give it a certificate with
// -tls-cert and -tls-key. NEXUS clients only use https:// addresses (except localhost).
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"nexus/accountd"
)

func main() {
	data := flag.String("data", "nexus-accountd-data", "folder for the account database")
	listen := flag.String("listen", "127.0.0.1:8480", "address to listen on")
	cert := flag.String("tls-cert", "", "TLS certificate file (optional)")
	key := flag.String("tls-key", "", "TLS key file (optional)")
	proxy := flag.Bool("trust-proxy", false, "take client addresses from X-Forwarded-For (only behind your own proxy)")
	flag.Parse()
	logger := log.New(os.Stderr, "nexus-accountd ", log.LstdFlags)
	s, err := accountd.New(accountd.Config{DataDir: *data, TrustProxy: *proxy, Logger: logger})
	if err != nil {
		logger.Fatal(err)
	}
	srv := &http.Server{Addr: *listen, Handler: s, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second}
	logger.Printf("listening on %s (protocol %d)", *listen, accountd.ProtocolVersion)
	if *cert != "" {
		err = srv.ListenAndServeTLS(*cert, *key)
	} else {
		err = srv.ListenAndServe()
	}
	logger.Fatal(err)
}
