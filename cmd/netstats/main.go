package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gochain-io/netstats"
	"github.com/gochain-io/netstats/geoip2"
)

const (
	DefaultTrustedPath = "trusted.json"
)

func main() {
	if err := run(os.Args[1:]); err == flag.ErrHelp {
		os.Exit(1)
	} else if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	defaultAddr := ":3000"
	if v := os.Getenv("PORT"); v != "" {
		defaultAddr = ":" + v
	}

	fs := flag.NewFlagSet("netstats", flag.ContinueOnError)
	addr := fs.String("addr", defaultAddr, "bind address")
	apiSecret := fs.String("api-secret", "", "api secret")
	trusted := fs.String("trusted", "", "trusted geo path")
	geoDBPath := fs.String("geodb", "", "MaxMind-City geo db path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Setup database.
	db := netstats.NewDB(os.Getenv("NETWORK_NAME"))

	// Read secret from environment variable, if blank.
	if *apiSecret == "" {
		*apiSecret = os.Getenv("WS_SECRET")
	}

	// Read trusted nodes set.
	geoByIP, err := readTrustedFile(*trusted)
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

	fmt.Printf("Listening on http://localhost%s\n", *addr)

	h := netstats.NewHandler()
	h.DB = db
	h.DB.GeoByIP = geoByIP
	h.APISecrets = strings.Split(*apiSecret, ",")
	return http.ListenAndServe(*addr, h)
}

func readTrustedFile(path string) (map[string]*netstats.Geo, error) {
	useDefault := path == ""
	if useDefault {
		path = DefaultTrustedPath
	}

	geoByIP := make(map[string]*netstats.Geo)
	if buf, err := ioutil.ReadFile(path); os.IsNotExist(err) && useDefault {
		log.Printf("No default trusted file found: %s\n", path)
		return geoByIP, nil
	} else if err != nil {
		return nil, err
	} else if err := json.Unmarshal(buf, &geoByIP); err != nil {
		return nil, err
	}
	log.Println("Loaded trusted nodes:")
	for ip, geo := range geoByIP {
		fmt.Printf("  %q: %#v\n", ip, geo)
	}
	return geoByIP, nil
}
