package main

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestIncAndCount(t *testing.T) {
	rateLimiter := newRaterLimiter()

	c := rateLimiter.GetCount("test.com")
	assert.Equal(t, c, uint(0), "incorrect count")

	c = rateLimiter.GetCount("a.com")
	assert.Equal(t, c, uint(0), "incorrect count")

	rateLimiter.Inc("test.com")
	c = rateLimiter.GetCount("test.com")
	assert.Equal(t, c, uint(1), "incorrect count")

	rateLimiter.Inc("test.com")
	c = rateLimiter.GetCount("test.com")
	assert.Equal(t, c, uint(2), "incorrect count")

	rateLimiter.Inc("a.com")
	c = rateLimiter.GetCount("a.com")
	assert.Equal(t, c, uint(1), "incorrect count")
}

func TestHourReset(t *testing.T) {
	rateLimiter := newRaterLimiter()

	getHour = func() int { return 0 }
	rateLimiter.Inc("test.com")
	rateLimiter.Inc("test.com")
	c := rateLimiter.GetCount("test.com")
	assert.Equal(t, c, uint(2), "incorrect count")

	getHour = func() int { return 1 }
	rateLimiter.Inc("test.com")
	c = rateLimiter.GetCount("test.com")
	assert.Equal(t, c, uint(1), "incorrect count")

	getHour = func() int { return 0 }
	rateLimiter.Inc("test.com")
	c = rateLimiter.GetCount("test.com")
	assert.Equal(t, c, uint(1), "incorrect count")
}
