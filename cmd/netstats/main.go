package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/blendle/zapdriver"
	"github.com/gochain-io/netstats"
	"github.com/gochain-io/netstats/geoip2"
	"go.uber.org/zap"
)

const (
	DefaultTrustedPath = "trusted.json"
)

func main() {
	start := time.Now()
	cfg := zapdriver.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "timestamp"
	lgr, err := cfg.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	if err := run(lgr, os.Args[1:]); err == flag.ErrHelp {
		os.Exit(1)
	} else if err != nil {
		lgr.Fatal("Fatal error", zap.Error(err), zap.Duration("runtime", time.Since(start)))
	}
	lgr.Info("Shutting down", zap.Duration("runtime", time.Since(start)))
}

func run(lgr *zap.Logger, args []string) error {
	defaultAddr := ":3000"
	if v := os.Getenv("PORT"); v != "" {
		defaultAddr = ":" + v
	}

	fs := flag.NewFlagSet("netstats", flag.ContinueOnError)
	addr := fs.String("addr", defaultAddr, "bind address")
	apiSecret := fs.String("api-secret", "", "api secret")
	trustedF := fs.String("trusted", "", "trusted geo path")
	strict := fs.Bool("strict", false, "enable strict mode to only allow trusted IPs")
	geoDBPath := fs.String("geodb", "", "MaxMind-City geo db path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Setup database.
	db := netstats.NewDB(os.Getenv("NETWORK_NAME"), lgr)

	// Read secret from environment variable, if blank.
	if *apiSecret == "" {
		*apiSecret = os.Getenv("WS_SECRET")
	}

	// Read trusted nodes set.
	trusted, err := readTrustedFile(lgr, *trustedF)
	if err != nil {
		return err
	}

	// Read trusted nodes set.
	if *geoDBPath != "" {
		s := &geoip2.GeoService{Path: *geoDBPath}
		if err := s.Open(); err != nil {
			return err
		}
		defer s.Close()
		db.GeoService = s
	}

	lgr.Info("HTTP server listening", zap.String("host", *addr))

	h := netstats.NewHandler(lgr)
	h.DB = db
	h.DB.Trusted = trusted
	h.DB.Strict = *strict
	h.APISecrets = strings.Split(*apiSecret, ",")
	return http.ListenAndServe(*addr, h)
}

func readTrustedFile(lgr *zap.Logger, path string) (netstats.GeoByIP, error) {
	useDefault := path == ""
	if useDefault {
		path = DefaultTrustedPath
	}

	geoByIP := make(netstats.GeoByIP)
	if buf, err := ioutil.ReadFile(path); os.IsNotExist(err) && useDefault {
		lgr.Info("No default trusted file found", zap.String("path", path))
		return geoByIP, nil
	} else if err != nil {
		return nil, err
	} else if err := json.Unmarshal(buf, &geoByIP); err != nil {
		return nil, err
	}

	lgr.Info("Loaded trusted nodes:", zap.Object("nodes", geoByIP))
	return geoByIP, nil
}
