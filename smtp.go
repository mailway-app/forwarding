package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mailway-app/config"
	"github.com/mailway-app/golib/rate"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type DomainStatus int

const (
	DOMAIN_UNCOMPLETE DomainStatus = 0
	DOMAIN_ACTIVE     DomainStatus = 1

	CRLF = "\r\n"
)

type Domain struct {
	Name   string       `json:"name"`
	Status DomainStatus `json:"status"`
}

const (
	// MAIL_STATUS_RECEIVED  = 0
	MAIL_STATUS_PROCESSED = 1
	// MAIL_STATUS_DELIVERED = 2
	MAIL_STATUS_SPAM = 3
)

var (
	LOOP_DETECTION_COUNT = 25
	RATE_LIMIT_COUNT     = 100
)

func hasLoop(email *Email) bool {
	return len(email.Data.Header["Received"]) > LOOP_DETECTION_COUNT
}

func runSpamassassin(file string) error {
	var cancel context.CancelFunc
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/usr/local/spamc.py", file)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to run spamc.py")
	}
	return nil
}

// https://www.iana.org/assignments/smtp-enhanced-status-codes/smtp-enhanced-status-codes.xhtml
var (
	unknownError    = errors.New("451 4.3.0 Internal server errror")
	parseError      = errors.New("451 4.5.2 Internal server errror")
	spamError       = errors.New("550 5.7.28 Our system has detected that this message is likely suspicious.")
	loopError       = errors.New("550 4.4.6 Routing error")
	processingError = errors.New("451 4.3.0 Internal server errror")
	configError     = errors.New("451 4.3.5 Internal server errror")
	rateError       = errors.New("450 4.4.2 Temporarily rate limited; suspicious behavior")

	rateLimiter = rate.NewRaterLimiter()
)

var (
	apiClient    *retryablehttp.Client
	mailDBClient *retryablehttp.Client
)

type Address struct {
	*mail.Address
	domain string
}

func parseAddress(v string) (*Address, error) {
	e, err := mail.ParseAddress(v)
	if err != nil {
		return nil, err
	}

	domain := strings.Split(e.Address, "@")[1]

	return &Address{
		e,
		domain,
	}, nil
}

func (s *session) makeMailHeader(rcptTo []string, mailFrom string) string {
	headers := []string{
		// Preserve SMTP Mail From and RCPT to
		"Mw-Int-Mail-From: " + mailFrom,
		// FIXME: for now only keep the first to
		"Mw-Int-Rcpt-To: " + rcptTo[0],
		"Mw-Int-Id: " + s.id.String(),
		"Mw-Int-Domain: " + s.domain.Name,
		"Mw-Int-Date: " + fmt.Sprintf("%d", time.Now().Unix()),
		"Mw-Int-Via: forwarding",
	}
	return strings.Join(headers, CRLF)
}

func (s *session) newBuffer() (*os.File, error) {
	name := fmt.Sprintf("%s/%s.eml", config.RUNTIME_LOCATION, s.id.String())
	log.Debugf("create file buffer %s", name)
	f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		log.Errorf("newBuffer: could not create temporary file: %s", err)
		return nil, unknownError
	}

	return f, nil
}

func (s *session) readBuffer() ([]byte, error) {
	name := fmt.Sprintf("%s/%s.eml", config.RUNTIME_LOCATION, s.id.String())
	data, err := ioutil.ReadFile(name)
	if err != nil {
		log.Errorf("readBuffer: could not read temporary file: %s", err)
		return nil, unknownError
	}
	return data, nil
}

func deleteBuffer(s *session) {
	name := fmt.Sprintf("%s/%s.eml", config.RUNTIME_LOCATION, s.id.String())
	log.Debugf("delete file buffer %s", name)
	if err := os.Remove(name); err != nil {
		log.Errorf("deleteBuffer: could not delete temporary file: %s", err)
	}
}

func getDomainRules(instance *config.Config, domain string) (DomainRules, error) {
	if config.CurrConfig.IsInstanceLocal() {
		return getLocalDomainRules(instance, domain)
	} else {
		return getAPIDomainRules(instance, domain)
	}
}

func getDomainConfig(instance *config.Config, domain string) (*Domain, error) {
	if config.CurrConfig.IsInstanceLocal() {
		return getLocalDomainConfig(instance, domain)
	} else {
		return getAPIDomainConfig(instance, domain)
	}
}

func rcptHandler(session *session, from string, to string) bool {
	e, err := parseAddress(to)
	if err != nil {
		log.Errorf("rcptHandler: failed to parse to: %s", err)
		return false
	}
	config, err := getDomainConfig(session.config, e.domain)
	if err != nil {
		log.Errorf("rcptHandler: failed to get domain config: %s", err)
		return false
	}
	if config == nil {
		log.Warnf("rcptHandler: domain %s not found", e.domain)
		return false
	}

	session.domain = config
	id, err := uuid.NewRandom()
	if err != nil {
		log.Errorf("rcptHandler: failed to generate uuid: %s", err)
		return false
	}
	session.id = id
	if err := mailDBNew(session, config.Name, id); err != nil {
		log.Errorf("mailDBNew: %s", err)
		return false
	}

	if err := mailDBSet(session, "to", to); err != nil {
		log.Errorf("mailDBSet to: %s", err)
		return false
	}
	if err := mailDBSet(session, "from", from); err != nil {
		log.Errorf("mailDBSet from: %s", err)
		return false
	}

	return config.Status == DOMAIN_ACTIVE
}

func logger(remoteIP, verb, line string) {
	log.Infof("%s %s %s", remoteIP, verb, line)
}

func Run(addr string) error {
	Debug = true
	srv := &Server{
		Addr:        addr,
		Handler:     mailHandler,
		HandlerRcpt: rcptHandler,
		Appname:     "fwdr",
		Hostname:    config.CurrConfig.InstanceHostname,
		Timeout:     5 * time.Minute,
		LogRead:     logger,
		LogWrite:    logger,
		MaxSize:     10485760,
	}

	if _, err := os.Stat(config.RUNTIME_LOCATION); os.IsNotExist(err) {
		if err := os.Mkdir(config.RUNTIME_LOCATION, 0700); err != nil {
			return errors.Wrap(err, "could not create runtime location")
		}
	}

	log.Infof("Forwarding listening on %s for %s (in mode %s)", addr, config.CurrConfig.InstanceHostname, config.CurrConfig.InstanceMode)
	return srv.ListenAndServe(config.CurrConfig)
}

type EmailEnvelope struct {
	From string
	To   []string
}
type Email struct {
	Envelope EmailEnvelope
	Data     *mail.Message
	// preserve the original email to avoid breaking any signatures
	Bytes []byte
}

func mailHandler(s *session, from string, to []string, data []byte) error {
	if rateLimiter.GetCount(s.domain.Name) > uint(RATE_LIMIT_COUNT) {
		log.Errorf("domain %s rate limited", s.domain.Name)
		return rateError
	}

	rateLimiter.Inc(s.domain.Name)

	if s.config.SpamFilter {
		log.Infof("run Spamassassin")

		file := fmt.Sprintf("%s/%s.eml", config.RUNTIME_LOCATION, s.id.String())
		if err := runSpamassassin(file); err != nil {
			log.Errorf("could not run spam filter: %s", err)
			return processingError
		}

		// read buffer again after Spamassassin wrote the status
		var err error
		data, err = s.readBuffer()
		if err != nil {
			log.Errorf("could not read buffer after spam processing: %s", err)
			return processingError
		}
	}

	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		log.Errorf("could not read message: %s", err)
		return parseError
	}

	if to := msg.Header.Get("to"); to != "" {
		if err := mailDBSet(s, "to", to); err != nil {
			log.Errorf("mailDBSet to failed: %s", err)
			return processingError
		}
	}
	if from := msg.Header.Get("from"); from != "" {
		if err := mailDBSet(s, "from", from); err != nil {
			log.Errorf("mailDBSet from failed: %s", err)
			return processingError
		}
	}

	if s.config.SpamFilter {
		spamStatus := msg.Header.Get("x-spam-status")
		if spamStatus == "" {
			log.Warn("x-spam-status is not present")
		} else {
			parts := strings.Split(spamStatus, ", ")
			isSpam := parts[0]
			score := parts[1]
			log.Infof("spam result: %s %s", isSpam, score)
			if isSpam == "Yes" {
				if err := mailDBUpdateMailStatus(s, MAIL_STATUS_SPAM); err != nil {
					log.Errorf("mailDBSet status failed: %s", err)
					return processingError
				}

				return spamError
			}
		}
	}

	email := Email{
		Envelope: EmailEnvelope{from, to},
		Data:     msg,
		Bytes:    data,
	}

	if hasLoop(&email) {
		log.Error("loop detected")
		return loopError
	}

	chans := MakeActionChans()
	domainRules, err := getDomainRules(s.config, s.domain.Name)
	if err != nil {
		log.Errorf("could not get domain's rules: %s", err)
		return configError
	}

	go func(domainRules DomainRules, email Email, chans ActionChans, s *session) {
		log.Debugf("running %d rule(s)", len(domainRules.Rules))
		ruleId, _ := ApplyRules(domainRules.Rules, email, chans)
		// No need to report the error because we also send it in the chans.error
		// channel
		if ruleId != nil {
			if err := mailDBUpdateMailStatus(s, MAIL_STATUS_PROCESSED); err != nil {
				log.Errorf("mailDBUpdateMailStatus: %s", err)
			}
			log.Debugf("rule %s was applied", *ruleId)
			if err := mailDBSet(s, "rule", string(*ruleId)); err != nil {
				log.Errorf("mailDBSet rule: %s", err)
			}
			chans.Close()
		}

	}(domainRules, email, chans, s)

	timeout := time.After(60 * time.Second)

	loop := true
	for loop {
		select {
		case drop := <-chans.drop:
			log.Infof("drop (by rule %t)", drop.DroppedRule)
			loop = false
			deleteBuffer(s)
		case send := <-chans.send:
			log.Infof("send to %s", send.To)
			if err := sendMailout(send.Email, send.To); err != nil {
				log.Errorf("error sending email out: %s", err)
				return processingError
			}
			loop = false
		case webhook := <-chans.webhook:
			log.Infof("call %s\n", webhook.Endpoint)
			if err := sendWebhook(webhook.Email, webhook.Endpoint, webhook.SecretToken); err != nil {
				log.Errorf("error sending webhook: %s", err)
				return processingError
			}
			loop = false
		case err := <-chans.error:
			log.Errorf("error during rule processing: %s", err)
			return processingError
		case <-timeout:
			chans.Error(errors.New("timed out"))
		}
	}

	return nil
}

func main() {
	if err := config.Init(); err != nil {
		log.Fatalf("failed to init config: %s", err)
	}

	if !config.CurrConfig.IsInstanceLocal() && config.CurrConfig.ServerJWT == "" {
		log.Fatal("server JWT is needed")
	}
	if v := config.CurrConfig.ForwardingLoopDetectionCount; v > 0 {
		LOOP_DETECTION_COUNT = v
	}
	if v := config.CurrConfig.ForwardingRateLimitingCount; v > 0 {
		RATE_LIMIT_COUNT = v
	}

	apiClient = retryablehttp.NewClient()
	apiClient.RetryMax = 5
	apiClient.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	mailDBClient = retryablehttp.NewClient()
	mailDBClient.RetryMax = 5
	mailDBClient.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	addr := fmt.Sprintf("127.0.0.1:%d", config.CurrConfig.PortForwarding)
	if err := Run(addr); err != nil {
		log.Fatal(err)
	}
}

func sendMailout(email Email, to string) error {
	from := email.Envelope.From
	addr := fmt.Sprintf("127.0.0.1:%d", config.CurrConfig.PortMailout)
	if err := smtp.SendMail(addr, nil, from, []string{to}, email.Bytes); err != nil {
		return errors.Wrap(err, "could not send email to mailout")
	}
	return nil
}

func sendWebhook(email Email, endpoint string, secretToken string) error {
	from := email.Envelope.From
	to := email.Envelope.To

	buffer := new(bytes.Buffer)

	buffer.WriteString(fmt.Sprintf("Mw-Int-Webhook-URL: %s%s", endpoint, CRLF))
	buffer.WriteString(fmt.Sprintf("Mw-Int-Webhook-Secret-Token: %s%s", secretToken, CRLF))
	buffer.Write(email.Bytes)

	addr := fmt.Sprintf("127.0.0.1:%d", config.CurrConfig.PortWebhook)
	if err := smtp.SendMail(addr, nil, from, to, buffer.Bytes()); err != nil {
		return errors.Wrap(err, "could not send email to webhook")
	}
	return nil
}
