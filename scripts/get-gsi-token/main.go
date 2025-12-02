package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()
}

func main() {
	var clientSecret string

	flag.StringVar(&clientSecret, "secret", "", "client secret")
	flag.Parse()

	log.Info().Msgf("Getting google ID token..")

	payload := []byte("client_id=" + CLIENT_ID + "&scope=email%20profile")
	req, err := http.NewRequest(
		http.MethodPost,
		"https://oauth2.googleapis.com/device/code",
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

	log.Info().Msg(string(body))

	var result TokenRequestResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Fatal().Msgf("err unmarshall resp body: %v", err)
	}

	log.Info().Msgf("Please COPY this code, and paste it in your browser: %v", result.UserCode)
	time.Sleep(5 * time.Second)
	log.Info().Msgf("Opening %v ...", result.VerificationURL)
	time.Sleep(1 * time.Second)

	openBrowser(result.VerificationURL)

	ggwp, err := pollResult("https://oauth2.googleapis.com/token", CLIENT_ID, clientSecret, result.DeviceCode, result.Interval)
	if err != nil {
		log.Fatal().Msgf("err poll: %v", err)
	}

	idToken := ggwp.IDToken

	log.Info().Msgf("Your id token is printed to the std output")
	fmt.Println(idToken)
}
