package main

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/mailway-app/config"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	if info == nil {
		return false
	}
	return !info.IsDir()
}

func getDomainConfigFile(domain string) string {
	return path.Join(config.ROOT_LOCATION, "domain.d", domain+".yaml")
}

func getLocalDomainConfig(instance *config.Config, domain string) (*Domain, error) {
	status := DOMAIN_UNCOMPLETE
	if fileExists(getDomainConfigFile(domain)) {
		status = DOMAIN_ACTIVE
	} else {
		log.Warnf("No configuration for domain %s not found", domain)
	}
	return &Domain{
		Name:   domain,
		Status: status,
	}, nil
}

func getLocalDomainRules(instance *config.Config, domain string) (DomainRules, error) {
	config := DomainRules{}

	file := getDomainConfigFile(domain)
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return config, errors.Wrap(err, "could not read domain config")
	}

	err = yaml.Unmarshal(content, &config)
	if err != nil {
		return config, errors.Wrap(err, "failed to parse")
	}

	// legalize uuid
	for i := range config.Rules {
		config.Rules[i].Id = RuleId(uuid.Nil.String())
	}

	return config, nil
}
