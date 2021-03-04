package main

import (
	"net/mail"
	"strconv"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/match"
)

type MatchType string
type MatchField string
type ActionType string

const (
	MATCH_ALL        MatchType = "all"
	MATCH_LITERAL    MatchType = "literal"
	MATCH_REGEX      MatchType = "regex"
	MATCH_TIME_AFTER MatchType = "timeAfter"

	FIELD_TO   MatchField = "to"
	FIELD_FROM MatchField = "from"
	FIELD_SUBJECT MatchField = "subject"

	ACTION_DROP    ActionType = "drop"
	ACTION_FORWARD ActionType = "forward"
	ACTION_WEBHOOK ActionType = "webhook"
)

type Match struct {
	Type  MatchType  `json:"type" yaml:"type"`
	Field MatchField `json:"field" yaml:"field"`
	Value string     `json:"value" yaml:"value"`
}

// For Webhook the Action value is: [endpoint, secret token]
type Action struct {
	Type  ActionType `json:"type" yaml:"type"`
	Value []string   `json:"value" yaml:"value"`
}
type RuleId string
type Rule struct {
	Id     RuleId   `json:"id" yaml:"id"`
	Type   int      `json:"type" yaml:"type"`
	Match  []Match  `json:"match" yaml:"match"`
	Action []Action `json:"action" yaml:"action"`
}

type DomainRules struct {
	Rules []Rule `json:"rules" yaml:"rules"`
}

type ActionDrop struct {
	DroppedRule bool
}

type ActionSend struct {
	Email Email
	To    string
}

type ActionWebhook struct {
	Email       Email
	Endpoint    string
	SecretToken string
}

type ActionChans struct {
	send    chan ActionSend
	drop    chan ActionDrop
	webhook chan ActionWebhook

	error chan error
}

func MakeActionChans() ActionChans {
	return ActionChans{
		send:    make(chan ActionSend),
		drop:    make(chan ActionDrop),
		webhook: make(chan ActionWebhook),
		error:   make(chan error),
	}
}

func (chans *ActionChans) Close() {
	close(chans.send)
	close(chans.drop)
	close(chans.webhook)
	close(chans.error)
}

func (chans *ActionChans) Error(e error) {
	chans.error <- e
	chans.Close()
}

func parseAddresses(v string) ([]string, error) {
	e, err := mail.ParseAddressList(v)
	if err != nil {
		return []string{}, errors.Wrapf(err, "failed to parse %s", v)
	}

	out := make([]string, len(e))
	for i, addr := range e {
		out[i] = addr.Address
	}
	return out, nil
}

func getField(field MatchField, email Email) ([]string, error) {

	switch field {
	case FIELD_TO:
		if to := email.Data.Header.Get("To"); to != "" {
			e, err := parseAddresses(to)
			if err != nil || len(e) == 0 {
				log.Warnf("failed to parse header `to` %s", to)
			} else {
				return e, nil
			}
		}
		// FIXME: support multiple to?
		to := email.Envelope.To[0]
		e, err := parseAddresses(to)
		if err != nil {
			return []string{},
				errors.Wrapf(err, "failed to envelope `to` %s", to)
		}
		return e, nil

	case FIELD_FROM:
		if from := email.Data.Header.Get("From"); from != "" {
			e, err := parseAddresses(from)
			if err != nil || len(e) == 0 {
				log.Warnf("failed to parse header `from` %s", from)
			} else {
				return e, nil
			}
		}
		e, err := parseAddresses(email.Envelope.From)
		if err != nil {
			return []string{},
				errors.Wrapf(err, "failed to envelope `from` %s", email.Envelope.From)
		}
		return e, nil

	case FIELD_SUBJECT:
        subject := email.Data.Header.Get("Subject")
		if subject != "" {
			e := []string{subject}
			return e, nil
		}
	}
	return []string{}, errors.Errorf("field %s not supported\n", field)
}

func HasMatch(predicates []Match, email Email) (bool, error) {
	for _, predicate := range predicates {
		switch predicate.Type {
		case MATCH_ALL:
			// ok
		case MATCH_TIME_AFTER:
			now := time.Now().UnixNano() / 1e6
			end, err := strconv.ParseInt(predicate.Value, 10, 64)
			if err != nil {
				return false, errors.Wrap(err, "could not parse int")
			}
			if end > now {
				log.Debugf("%d > %d", end, now)
				return false, nil
			} else {
				log.Debugf("%d < %d", end, now)
			}
		case MATCH_REGEX:
			vs, err := getField(predicate.Field, email)
			if err != nil {
				return false, errors.Wrap(err, "failed to match regex")
			}
			for _, v := range vs {
				if !match.Match(v, predicate.Value) {
					log.Debugf("%s != %s", v, predicate.Value)
					return false, nil
				} else {
					log.Debugf("%s ~= %s", v, predicate.Value)
					// once matched, exit the loop now
					break
				}
			}
		case MATCH_LITERAL:
			vs, err := getField(predicate.Field, email)
			if err != nil {
				return false, errors.Wrap(err, "failed to match literal")
			}
			for _, v := range vs {
				if v != predicate.Value {
					log.Debugf("%s != %s", v, predicate.Value)
					return false, nil
				} else {
					log.Debugf("%s == %s", v, predicate.Value)
					// once matched, exit the loop now
					break
				}
			}

		default:
			return false, errors.Errorf("action %s isn't supported\n", predicate)
		}

	}
	return true, nil
}

func ApplyRules(rules []Rule, email Email, chans ActionChans) (*RuleId, error) {
	for _, rule := range rules {
		match, err := HasMatch(rule.Match, email)
		if err != nil {
			chans.Error(err)
			return nil, err
		}
		if match {
			for _, action := range rule.Action {
				switch action.Type {
				case ACTION_DROP:
					chans.drop <- ActionDrop{DroppedRule: true}
				case ACTION_WEBHOOK:
					if len(action.Value) != 2 {
						return nil, errors.Errorf(
							"invalid webhook configuration, expected 2 params got %d", len(action.Value))
					}
					chans.webhook <- ActionWebhook{
						Email:       email,
						Endpoint:    action.Value[0],
						SecretToken: action.Value[1],
					}
				case ACTION_FORWARD:
					for _, to := range action.Value {
						chans.send <- ActionSend{Email: email, To: to}
					}
				default:
					e := errors.Errorf("action %s isn't supported\n", action)
					chans.Error(e)
					return nil, e
				}
			}
			return &rule.Id, nil
		}
	}

	chans.drop <- ActionDrop{DroppedRule: false}
	return nil, nil
}
