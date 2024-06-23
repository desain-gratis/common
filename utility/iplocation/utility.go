package iplocation

import (
	"context"
)

var Default Provider

type Provider interface {
	Get(ctx context.Context, ip string) (*IPLocation, error)
}

type IPLocation struct {
	IP           string `json:"ip"`
	IPVersion    string `json:"ip_version"`
	CountryName  string `json:"country_name"`
	CountryCode2 string `json:"country_code2"`
	ISP          string `json:"isp"`
	City         string `json:"city"`
}

func Get(ctx context.Context, ip string) (*IPLocation, error) {
	if Default == nil {
		return nil, nil
	}
	return Default.Get(ctx, ip)
}
