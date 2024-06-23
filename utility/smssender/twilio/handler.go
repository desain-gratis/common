package twilio

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
)

type twilioSMSHandler struct {
	accountSID   string
	authToken    string
	twilioNumber string
	endpoint     string
	client       *http.Client
}

func New(
	accountSID string,
	authToken string,
	twilioNumber string,
	endpoint string,
	client *http.Client,
) *twilioSMSHandler {
	return &twilioSMSHandler{
		accountSID:   accountSID,
		authToken:    authToken,
		twilioNumber: twilioNumber,
		endpoint:     endpoint,
		client:       client,
	}
}

func (t *twilioSMSHandler) Send(ctx context.Context, phoneNumber string, payload string) error {
	v := url.Values{}
	v.Set("To", phoneNumber)
	v.Set("From", t.twilioNumber)
	v.Set("Body", payload)
	rb := *strings.NewReader(v.Encode())

	req, _ := http.NewRequest("POST", t.endpoint, &rb)
	req.SetBasicAuth(t.accountSID, t.authToken)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Make request
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}

	// TODO add util for status callback handler ( if sent, etc.), return channel with default timeout; goroutine pooling
	//      (eg. you can't send new sms request if the previous sequence is not yet finished)
	//      wait until get feedback DELIVERED
	//      https://www.twilio.com/docs/sms/api/message-resource#message-status-values

	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	log.Trace().Msgf("%+v\n", string(response))

	var data map[string]any
	err = json.Unmarshal(response, &data)
	if err != nil {
		return err
	}

	if _, ok := data["status"]; !ok {
		return errors.New("Status response not foundd")
	}

	status, ok := data["status"].(string)
	if !ok {
		return errors.New("Status response not found")
	}

	if status != "queued" {
		err := errors.New("SMS not queued")
		log.Err(err).Msgf("%+v\n", string(response))
		return err
	}

	return nil
}
