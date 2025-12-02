package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/rs/zerolog/log"
)

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
		args = []string{url}
	}

	command := exec.Command(cmd, args...)
	err := command.Start()
	if err != nil {
		return err
	}

	return nil
}

func pollResult(pollUrl, clientId, clientSecret, deviceCode string, intervalSeconds int) (*TokenPollResponse, error) {
	if intervalSeconds == 0 {
		intervalSeconds = 5
	}

	payload := []byte("client_id=" + clientId + "&client_secret=" + clientSecret + "&code=" + deviceCode + "&grant_type=http://oauth.net/grant_type/device/1.0")

	for i := 0; i < 100; i++ {
		req, err := http.NewRequest(
			http.MethodPost,
			pollUrl,
			bytes.NewReader(payload),
		)
		if err != nil {
			log.Fatal().Msgf("err build http req: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatal().Msgf("err get http resp: %v", err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal().Msgf("err get get body: %v", err)
		}

		var result TokenPollResponse
		err = json.Unmarshal(body, &result)
		if err != nil {
			log.Fatal().Msgf("err unmarshall resp body: %v", err)
		}

		if result.Error == nil {
			return &result, nil
		}

		log.Debug().Msgf("poll: %v %v", i+1, string(body))

		time.Sleep(time.Duration(intervalSeconds) * time.Second)
	}

	return nil, errors.New("exceed retry limit")
}
