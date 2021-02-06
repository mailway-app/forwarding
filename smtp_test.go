package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoop(t *testing.T) {
	email := makeEmail(`Received: from localhost (localhost [127.0.0.1]) by test1.smtp-in.mailway.app (mailout) with SMTP for <a@gmail.com>; Fri,
  5 Feb 2021 19:01:27 +0000 (UTC)
Received: from mail.yahoo.com (mail.yahoo.com. [77.238.177.146]) by test1.smtp-in.mailway.app (fwdr) with SMTP for <u@example.com>; Fri,
  5 Feb 2021 19:01:27 +0000 (UTC)
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
From: sven@b.ee
To: sven@gmail.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)
	assert.True(t, hasLoop(&email))
}

func TestNoLoop(t *testing.T) {
	email := makeEmail(`Received: from localhost (localhost [127.0.0.1]) by test1.smtp-in.mailway.app (mailout) with SMTP for <a@gmail.com>; Fri,
  5 Feb 2021 19:01:27 +0000 (UTC)
Received: from mail.yahoo.com (mail.yahoo.com. [77.238.177.146]) by test1.smtp-in.mailway.app (fwdr) with SMTP for <u@example.com>; Fri,
  5 Feb 2021 19:01:27 +0000 (UTC)
Received: from sonic.gate.mail.ne1.yahoo.com by sonic314.consmr.mail.ir2.yahoo.com with HTTP; Fri, 5 Feb 2021 19:01:27 +0000
From: sven@b.ee
To: sven@gmail.com
Subject: test
Date: Sun, 8 Jan 2017 20:37:44 +0200

Hello world!
	`)
	assert.False(t, hasLoop(&email))
}
