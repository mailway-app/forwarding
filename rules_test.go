package main

import (
	"bytes"
	"fmt"
	"net/mail"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func makeEmail(body string) Email {
	msg, err := mail.ReadMessage(bytes.NewReader([]byte(body)))
	if err != nil {
		panic(err)
	}

	from := msg.Header.Get("From")
	to := msg.Header.Get("To")
	return Email{
		Envelope: EmailEnvelope{from, []string{to}},
		Data:     msg,
	}
}

func makeEmailWithEnvelope(body, to, from string) Email {
	msg, err := mail.ReadMessage(bytes.NewReader([]byte(body)))
	if err != nil {
		panic(err)
	}
	return Email{
		Envelope: EmailEnvelope{from, []string{to}},
		Data:     msg,
	}
}

func TestDropByDefault(t *testing.T) {
	rules := []Rule{}
	chans := MakeActionChans()
	email := makeEmail(`From: sven@b.ee
To: sven@gmail.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	go func() {
		_, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
	}()
	assert.Equal(t, <-chans.drop, ActionDrop{DroppedRule: false}, "Mail was not dropped")
}

func TestDropMatchAll(t *testing.T) {
	rules := []Rule{
		{
			Match: []Match{
				{Type: MATCH_ALL},
			},
			Action: []Action{
				{Type: ACTION_DROP},
			},
		},
	}
	chans := MakeActionChans()
	email := makeEmail(`From: sven@b.ee
To: sven@gmail.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	go func() {
		_, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
	}()
	assert.Equal(t, <-chans.drop, ActionDrop{DroppedRule: true}, "Mail was not dropped")
}

func TestForwardMatchAll(t *testing.T) {
	rules := []Rule{
		{
			Match: []Match{
				{Type: MATCH_ALL},
			},
			Action: []Action{
				{Type: ACTION_FORWARD, Value: []string{"me"}},
			},
		},
	}
	chans := MakeActionChans()
	email := makeEmail(`From: sven@b.ee
To: sven@gmail.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	go func() {
		_, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
	}()
	assert.Equal(t, <-chans.send, ActionSend{To: "me", Email: email}, "Mail was not dropped")
}

func TestForwardMultipleToMatchAll(t *testing.T) {
	rules := []Rule{
		{
			Match: []Match{
				{Type: MATCH_ALL},
			},
			Action: []Action{
				{Type: ACTION_FORWARD, Value: []string{"a", "b"}},
			},
		},
	}
	chans := MakeActionChans()
	email := makeEmail(`From: sven@b.ee
To: sven@gmail.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	go func() {
		_, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
	}()
	assert.Equal(t, <-chans.send, ActionSend{To: "a", Email: email}, "Mail was not sent")
	assert.Equal(t, <-chans.send, ActionSend{To: "b", Email: email}, "Mail was not sent")
}

func TestRespectRuleOrder(t *testing.T) {
	rules := []Rule{
		{
			Match: []Match{
				{Type: MATCH_ALL},
			},
			Action: []Action{
				{Type: ACTION_DROP},
			},
		},
		{
			Match: []Match{
				{Type: MATCH_ALL},
			},
			Action: []Action{
				{Type: ACTION_FORWARD, Value: []string{"me"}},
			},
		},
	}
	chans := MakeActionChans()
	email := makeEmail(`From: sven@b.ee
To: sven@gmail.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	go func() {
		_, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
	}()
	assert.Equal(t, <-chans.drop, ActionDrop{DroppedRule: true}, "Mail was not dropped")
}

func TestMatchFieldTo(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: a@gmail.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_TO, Value: "a@gmail.com"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_LITERAL, Field: FIELD_TO, Value: "u@gmail.com"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, false)
}

func TestMatchFieldToNoHeader(t *testing.T) {
	email := makeEmailWithEnvelope(`From: sven@b.ee
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`, "a@gmail.com", "sven@b.ee")

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_TO, Value: "a@gmail.com"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_LITERAL, Field: FIELD_TO, Value: "u@gmail.com"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, false)
}

func TestMatchFieldToWithName(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: Tom <mail@jack.uk>
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_TO, Value: "mail@jack.uk"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)
}

func TestMatchFieldFrom(t *testing.T) {
	email := makeEmail(`From: a@gmail.com
To: sven@b.ee
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_FROM, Value: "a@gmail.com"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_LITERAL, Field: FIELD_FROM, Value: "u@gmail.com"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, false)
}

func TestMatchFieldFromNoHeader(t *testing.T) {
	email := makeEmailWithEnvelope(`Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`, "a@gmail.com", "sven@b.ee")

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_TO, Value: "a@gmail.com"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_LITERAL, Field: FIELD_TO, Value: "u@gmail.com"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, false)
}

func TestMatchFieldFromWithName(t *testing.T) {
	email := makeEmail(`To: sven@b.ee
From: mail <mail@jack.uk>
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_FROM, Value: "mail@jack.uk"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)
}

func TestMatchRegexFrom(t *testing.T) {
	email := makeEmail(`To: sven@b.ee
From: abc@test.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_REGEX, Field: FIELD_FROM, Value: "*@test.com"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_REGEX, Field: FIELD_FROM, Value: "abc@*.com"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_REGEX, Field: FIELD_FROM, Value: "abc@test.*"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_REGEX, Field: FIELD_FROM, Value: "u*@test.com"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, false)
}

func TestMatchRegexTo(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: abc@test.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_REGEX, Field: FIELD_TO, Value: "*@test.com"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_REGEX, Field: FIELD_TO, Value: "abc@*.com"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_REGEX, Field: FIELD_TO, Value: "abc@test.*"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_REGEX, Field: FIELD_TO, Value: "u*@test.com"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, false)
}

func TestRunActionAfterTimePassed(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: abc@test.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	nowMs := time.Now().UnixNano() / 1e6

	rules := []Rule{
		{
			Match: []Match{
				{Type: MATCH_TIME_AFTER, Value: fmt.Sprintf("%d", nowMs-3600000)},
			},
			Action: []Action{
				{Type: ACTION_FORWARD, Value: []string{"me"}},
			},
		},
	}
	chans := MakeActionChans()

	go func() {
		_, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
	}()
	assert.Equal(t, <-chans.send, ActionSend{Email: email, To: "me"}, "Mail was not forwarded")
}

func TestRunNotActionAfterTimePassed(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: abc@test.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	nowMs := time.Now().UnixNano() / 1e6

	rules := []Rule{
		{
			Match: []Match{
				{Type: MATCH_TIME_AFTER, Value: fmt.Sprintf("%d", nowMs+3600000)},
			},
			Action: []Action{
				{Type: ACTION_FORWARD, Value: []string{"me"}},
			},
		},
	}
	chans := MakeActionChans()

	go func() {
		_, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
	}()
	assert.Equal(t, <-chans.drop, ActionDrop{DroppedRule: false}, "Mail was not dropped")
}

func TestFirstRuleMatchesStop(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: abc@test.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	rules := []Rule{
		{
			Id: "1",
			Match: []Match{
				{Type: MATCH_LITERAL, Field: FIELD_FROM, Value: "a"},
			},
			Action: []Action{
				{Type: ACTION_DROP},
			},
		},
		{
			Id: "2",
			Match: []Match{
				{Type: MATCH_ALL},
			},
			Action: []Action{
				{Type: ACTION_FORWARD, Value: []string{"me"}},
			},
		},
		{
			Id: "3",
			Match: []Match{
				{Type: MATCH_LITERAL, Field: FIELD_FROM, Value: "a"},
			},
			Action: []Action{
				{Type: ACTION_DROP},
			},
		},
	}
	chans := MakeActionChans()

	go func() {
		ruleId, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
		assert.Equal(t, *ruleId, RuleId("2"), "matched ruleId is incorrect")
	}()
	assert.Equal(t, <-chans.send, ActionSend{Email: email, To: "me"}, "Mail was not forwarded")
}

func TestCallMultipleActions(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: abc@test.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	rules := []Rule{
		{
			Match: []Match{
				{Type: MATCH_ALL},
			},
			Action: []Action{
				{Type: ACTION_FORWARD, Value: []string{"a"}},
				{Type: ACTION_FORWARD, Value: []string{"b"}},
				{Type: ACTION_DROP},
			},
		},
	}
	chans := MakeActionChans()

	go func() {
		_, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
	}()
	assert.Equal(t, <-chans.send, ActionSend{Email: email, To: "a"}, "Mail was not forwarded")
	assert.Equal(t, <-chans.send, ActionSend{Email: email, To: "b"}, "Mail was not forwarded")
	assert.Equal(t, <-chans.drop, ActionDrop{DroppedRule: true}, "Mail was not dropped")
}

func TestRunWebhookAction(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: abc@test.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	rules := []Rule{
		{
			Match: []Match{
				{Type: MATCH_ALL},
			},
			Action: []Action{
				{Type: ACTION_WEBHOOK, Value: []string{"https://a", "secret_token"}},
			},
		},
	}
	chans := MakeActionChans()

	go func() {
		_, err := ApplyRules(rules, email, chans)
		assert.Equal(t, err, nil, "ApplyRules return an error")
	}()
	assert.Equal(t, <-chans.webhook,
		ActionWebhook{Email: email, Endpoint: "https://a", SecretToken: "secret_token"}, "webhook was not called")
}

func TestMatchFieldMultipleTo(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: Tom <mail@jack.uk>, Ana <mail@ana.uk>
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_TO, Value: "mail@jack.uk"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)
}

func TestNoMatchedRule(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: abc@test.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	rules := []Rule{
		{
			Match: []Match{
				{Type: MATCH_LITERAL, Field: FIELD_TO, Value: "abc"},
			},
			Action: []Action{},
		},
	}
	chans := MakeActionChans()

	go func() {
		rule, err := ApplyRules(rules, email, chans)
		assert.Nil(t, err)
		assert.Nil(t, rule)
	}()
}

func TestMatchFromWithBrokenHeader(t *testing.T) {
	email := makeEmailWithEnvelope(`From: a
To: Ana <mail@ana.uk>
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`, "to@mailway.app", "from@mailway.app")

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_FROM, Value: "from@mailway.app"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)
}

func TestMatchFieldSubject(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: a@gmail.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_SUBJECT, Value: "test"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_LITERAL, Field: FIELD_SUBJECT, Value: "not"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, false)
}

func TestMatchFieldWithEmptySubject(t *testing.T) {
	email := makeEmail(`From: sven@b.ee
To: a@gmail.com
Subject:
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_LITERAL, Field: FIELD_SUBJECT, Value: ""},
	}

	go func() {
		_, err := HasMatch(matches, email)
		assert.Equal(t, err, nil, "field subject not supported")
	}()

	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)
}

func TestMatchRegexSubject(t *testing.T) {
	email := makeEmail(`To: sven@b.ee
From: abc@abc.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)

	matches := []Match{
		{Type: MATCH_REGEX, Field: FIELD_SUBJECT, Value: "*st"},
	}
	v, err := HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, true)

	matches = []Match{
		{Type: MATCH_REGEX, Field: FIELD_SUBJECT, Value: "no*"},
	}
	v, err = HasMatch(matches, email)
	assert.Nil(t, err)
	assert.Equal(t, v, false)
}
