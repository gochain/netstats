package geoip2

import (
	"context"
	"net"
	"strconv"

	"github.com/gochain-io/netstats"
	"github.com/oschwald/geoip2-golang"
)

// Ensure implementation implements interface.
var _ (netstats.GeoService) = (*GeoService)(nil)

// GeoService is a service for looking up geolocation by IP address using a MaxMind database.
//
// https://dev.maxmind.com/geoip/geoip2/geolite2/
type GeoService struct {
	r *geoip2.Reader

	// Filename of the MaxMind city database ("GeoIP2-City.mmdb").
	Path string
}

// Open initializes the internal reader with the service's file path.
func (s *GeoService) Open() (err error) {
	if s.r, err = geoip2.Open(s.Path); err != nil {
		return err
	}
	return nil
}

// Close closes the internal reader, if set.
func (s *GeoService) Close() error {
	if s.r != nil {
		s.r.Close()
	}
	return nil
}

// GeoByIP returns geolocation information by IP address.
func (s *GeoService) GeoByIP(ctx context.Context, ip string) (*netstats.Geo, error) {
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return nil, netstats.ErrIPNotFound
	}

	record, err := s.r.City(ipAddr)
	if err != nil {
		return nil, err
	}

	geo := &netstats.Geo{
		City:    record.City.Names["en"],
		Country: record.Country.IsoCode,
		LL:      []float64{record.Location.Latitude, record.Location.Longitude},
		Metro:   int(record.Location.MetroCode),
		Range:   []int{0, 0},
	}

	geo.Zip, _ = strconv.Atoi(record.Postal.Code)

	if len(record.Subdivisions) > 0 {
		geo.Region = record.Subdivisions[0].Names["en"]
	}
	return geo, nil
}
