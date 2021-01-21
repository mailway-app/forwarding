package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/mailway-app/config"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	API_BASE_URL = "https://apiv1.mailway.app"
)

type Domain struct {
	Name      string       `json:"name"`
	MxRecords []string     `json:"mxRecords"`
	Status    DomainStatus `json:"status"`
	// FIXME: we can get rules from domain call
}

type APIResponse struct {
	Ok    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

func getDomainConfig(instance *config.Config, domain string) (*Domain, error) {
	url := fmt.Sprintf("%s/instance/%s/domain/%s", API_BASE_URL, instance.ServerId, domain)
	log.Debugf("request to %s", url)

	req, err := retryablehttp.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+instance.ServerJWT)
	req.Header.Set("User-Agent", "fwdr")

	res, getErr := apiClient.Do(req)
	if getErr != nil {
		return nil, errors.Wrap(getErr, "could not send request")
	}

	if res.StatusCode == 200 {
		if res.Body != nil {
			defer res.Body.Close()
		}

		body, readErr := ioutil.ReadAll(res.Body)
		if readErr != nil {
			return nil, errors.Wrap(readErr, "couldn't read body")
		}

		var d APIResponse
		jsonErr := json.Unmarshal(body, &d)
		if jsonErr != nil {
			return nil, errors.Wrap(jsonErr, "failed to parse API envelope")
		}
		if !d.Ok {
			return nil, errors.Errorf("API failed with: %s", d.Error)
		}

		var c Domain
		jsonErr = json.Unmarshal(d.Data, &c)
		if jsonErr != nil {
			return nil, errors.Wrap(jsonErr, "failed to parse API envelope")
		}
		return &c, nil
	}

	switch res.StatusCode {
	case 404:
		return nil, nil
	default:
		return nil, errors.Errorf("unexpected response from API: %s", res.Status)
	}
}

func getDomainRules(instance *config.Config, domain string) (DomainRules, error) {
	var domainRules DomainRules
	url := fmt.Sprintf("%s/instance/%s/domain/%s/rules", API_BASE_URL, instance.ServerId, domain)
	log.Debugf("request to %s", url)
	req, err := retryablehttp.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return domainRules, err
	}

	req.Header.Set("User-Agent", "smtp")
	req.Header.Set("Authorization", "Bearer "+instance.ServerJWT)

	res, getErr := apiClient.Do(req)
	if getErr != nil {
		return domainRules, err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return domainRules, err
	}

	var d APIResponse
	jsonErr := json.Unmarshal(body, &d)
	if jsonErr != nil {
		return domainRules, errors.Wrap(jsonErr, "failed to parse API envelope")
	}
	if !d.Ok {
		return domainRules, errors.Errorf("API failed with: %s", d.Error)
	}

	jsonErr = json.Unmarshal(d.Data, &domainRules)
	if jsonErr != nil {
		return domainRules, errors.Wrap(jsonErr, "failed to parse API envelope")
	}
	return domainRules, nil
}
