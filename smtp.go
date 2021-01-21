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

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	LOCAL_SMTP = "localhost:2525"

	// MAIL_STATUS_RECEIVED  = 0
	MAIL_STATUS_PROCESSED = 1
	// MAIL_STATUS_DELIVERED = 2
	MAIL_STATUS_SPAM = 3
)

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

	rateLimiter = newRaterLimiter()
)

var (
	apiClient    *retryablehttp.Client
	mailDBClient *retryablehttp.Client
)

type DomainStatus int

const (
	// DOMAIN_UNCOMPLETE DomainStatus = 0
	DOMAIN_ACTIVE DomainStatus = 1
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

		"X-Mailway-Id: " + s.id.String(),
		"X-Mailway-Domain: " + s.domain.Name,
		"Autoforwarded: true",
	}
	return strings.Join(headers, "\r\n")
}

func (s *session) newBuffer() (*os.File, error) {
	name := "/tmp/" + s.id.String() + ".eml"
	log.Debugf("create file buffer %s", name)
	f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		log.Errorf("newBuffer: could not create temporary file: %s", err)
		return nil, unknownError
	}

	return f, nil
}

func (s *session) readBuffer() ([]byte, error) {
	data, err := ioutil.ReadFile("/tmp/" + s.id.String() + ".eml")
	if err != nil {
		log.Errorf("readBuffer: could not read temporary file: %s", err)
		return nil, unknownError
	}
	return data, nil
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
	if err := mailDBNew(config.Name, id); err != nil {
		log.Errorf("mailDBNew: %s", err)
		return false
	}

	if err := mailDBSet(session, "to", to); err != nil {
		log.Errorf("mailDBSet to: %s", err)
		return false
	}
	if err := mailDBSet(session, "from", to); err != nil {
		log.Errorf("mailDBSet from: %s", err)
		return false
	}

	return config.Status == DOMAIN_ACTIVE
}

func logger(remoteIP, verb, line string) {
	log.Infof("%s %s %s", remoteIP, verb, line)
}

func Run(addr string, config *config.Config) error {
	Debug = true
	srv := &Server{
		Addr:        addr,
		Handler:     mailHandler,
		HandlerRcpt: rcptHandler,
		Appname:     "fwdr",
		Hostname:    config.InstanceHostname,
		Timeout:     60 * time.Second,
		LogRead:     logger,
		LogWrite:    logger,
	}

	log.Infof("Forwarding listening on %s for %s", addr, config.InstanceHostname)
	return srv.ListenAndServe(config)
}

type EmailEnvelope struct {
	From string
	To   []string
}
type Email struct {
	Envelope EmailEnvelope
	Data     *mail.Message
}

func (e Email) String() string {
	headers := make([]string, 0)
	for k, vs := range e.Data.Header {
		for _, v := range vs {
			headers = append(headers, fmt.Sprintf("%s: %s", k, v))
		}
	}

	out := strings.Join(headers, "\r\n")
	out += "\r\n\r\n"

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(e.Data.Body); err != nil {
		log.Errorf("buffer ReadFrom error: %s", err)
	}
	out += buf.String()

	return out
}

func mailHandler(s *session, from string, to []string, data []byte) error {
	if rateLimiter.GetCount(s.domain.Name) > 60 {
		log.Infof("domain %s rate limited", s.domain.Name)
		return rateError
	}

	rateLimiter.Inc(s.domain.Name)

	if s.config.SpamFilter {
		log.Infof("run Spamassassin")
		file := "/tmp/" + s.id.String() + ".eml"
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

	// prevents loop for now, since ReadMessage doesn't support multiple times
	// the same header key we will just count the substring.
	// TODO: implement a way to resolve loops by flattening the chain
	if strings.Count(string(data), "X-Mailway-Id:") > 1 {
		log.Warn("loop detected")
		return loopError
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
	}

	chans := MakeActionChans()
	domainRules, err := getDomainRules(s.config, s.domain.Name)
	if err != nil {
		log.Errorf("could not get domain's rules: %s", err)
		return configError
	}

	go func(domainRules DomainRules, email Email, chans ActionChans, s *session) {
		log.Debugf("running %d rule(s)", len(domainRules.Rules))
		ruleId, err := ApplyRules(domainRules.Rules, email, chans)
		if err == nil {
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
			log.Debugf("drop (by rule %t)", drop.DroppedRule)
			loop = false
		case send := <-chans.send:
			log.Debugf("send to %s", send.To)
			if err := sendMail(send.Email, send.To); err != nil {
				log.Errorf("error sending email: %s", err)
				return processingError
			}
			loop = false
		case <-chans.accept:
			log.Debugf("accept\n")
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
	log.SetLevel(log.DebugLevel)

	c, err := config.Read()
	if err != nil {
		panic(err)
	}

	if c.ServerJWT == "" {
		panic("server JWT is needed")
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

	addr := fmt.Sprintf("127.0.0.1:%d", c.PortForwarding)
	if err := Run(addr, c); err != nil {
		panic(err)
	}
}

func sendMail(email Email, to string) error {
	from := email.Envelope.From
	data := email.String()
	if err := smtp.SendMail(LOCAL_SMTP, nil, from, []string{to}, []byte(data)); err != nil {
		return errors.Wrap(err, "could not send email")
	}
	return nil
}
