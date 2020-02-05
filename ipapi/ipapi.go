package ipapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gochain-io/netstats"
)

const url = "http://ip-api.com/json/"

// DefaultClient is backed by ip-api.com.
var DefaultClient = NewClient(url)

// Client implements netstats.GeoService.
type Client struct {
	url string
}

func NewClient(url string) *Client {
	return &Client{url: url}
}

// Close closes the internal reader, if set.
func (s *Client) Close() error {
	return nil
}

// GeoByIP returns geolocation information by IP address.
func (s *Client) GeoByIP(ctx context.Context, ip string) (*netstats.Geo, error) {
	type ipApiGeo struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Country string `json:"country"`
		//CountryCode string      `json:"countryCode"`
		Region string `json:"region"`
		//RegionName  string      `json:"regionName"`
		City string      `json:"city"`
		Lat  json.Number `json:"lat"`
		Long json.Number `json:"long"`
	}
	resp, err := http.Get(s.url + ip)
	if err != nil {
		return nil, err
	}
	if resp.Body == nil {
		return nil, netstats.ErrIPNotFound
	}
	var geo ipApiGeo
	if err := json.NewDecoder(resp.Body).Decode(&geo); err != nil {
		return nil, fmt.Errorf("failed to decode ip-api.com response: %v", err)
	}
	if geo.Status != "success" {
		return nil, fmt.Errorf("unsuccessful ip-api.com query: %s: %s", geo.Status, geo.Message)
	}
	return &netstats.Geo{
		City:    geo.City,
		Country: geo.Country,
		LL:      [2]json.Number{geo.Lat, geo.Long},
		Region:  geo.Region,
	}, nil
}
