package iplocationnet

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/desain-gratis/common/utility/iplocation"
)

const host = "https://api.iplocation.net"

var _ iplocation.Provider = &handler{}

type handler struct{}

func New() *handler {
	return &handler{}
}

func (h *handler) Get(ctx context.Context, ip string) (*iplocation.IPLocation, error) {
	if ip == "" {
		return nil, nil
	}

	u, err := url.Parse(host)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Add("ip", ip)
	u.RawQuery = q.Encode()
	url := u.String()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// todo not use defaul client
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	// todo: protect
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var _result map[string]interface{}
	err = json.Unmarshal(payload, &_result)
	if err != nil {
		return nil, err
	}

	// todo: proper error handling
	v, _ := _result["ip_version"].(float64)
	var result iplocation.IPLocation
	result.CountryCode2, _ = _result["country_code2"].(string)
	result.CountryName, _ = _result["country_name"].(string)
	result.IP, _ = _result["ip"].(string)
	result.IPVersion = strconv.Itoa(int(v))
	result.ISP, _ = _result["isp"].(string)

	return &result, nil
}
