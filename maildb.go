package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"
)

const (
	MAILDB_BASE_URI = "http://127.0.0.1:8081/db"
)

func mailDBNew(domain string, uuid uuid.UUID) error {
	log.Debugf("mailDB: create new email %s", uuid)
	url := fmt.Sprintf("%s/domain/%s/new/%s", MAILDB_BASE_URI, domain, uuid.String())
	body := ""
	req, err := retryablehttp.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}

	_, err = mailDBClient.Do(req)
	if err != nil {
		return err
	}

	return nil

}

func mailDBUpdateMailStatus(s *session, status int) error {
	log.Debugf("mailDB: update status %s %d", s.id, status)
	url := fmt.Sprintf("%s/domain/%s/update/%s", MAILDB_BASE_URI, s.domain.Name, s.id.String())
	body := fmt.Sprintf("{\"status\":%d}", status)
	req, err := retryablehttp.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		return err
	}

	_, err = mailDBClient.Do(req)
	if err != nil {
		return err
	}

	return nil
}

func mailDBSet(s *session, field string, value string) error {
	log.Debugf("mailDB: update %s %s %s", field, s.id, value)
	url := fmt.Sprintf("%s/domain/%s/update/%s", MAILDB_BASE_URI, s.domain.Name, s.id.String())
	body := fmt.Sprintf("{\"%s\":\"%s\"}", field, value)
	req, err := retryablehttp.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		return err
	}

	_, err = mailDBClient.Do(req)
	if err != nil {
		return err
	}

	return nil
}
