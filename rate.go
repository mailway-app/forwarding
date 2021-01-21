package main

import (
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	// declare as var to be able to replace it in testing
	getHour = func() int {
		hour, _, _ := time.Now().Clock()
		return hour
	}
)

type RateLimiter struct {
	currHour int
	// 24-hours array of mapping domain to count
	hourCountsByDomain []map[string]uint
}

func newRaterLimiter() *RateLimiter {
	hourCountsByDomain := make([]map[string]uint, 24)

	// init first hour
	h := getHour()
	hourCountsByDomain[h] = make(map[string]uint)

	return &RateLimiter{
		hourCountsByDomain: hourCountsByDomain,
		currHour:           h,
	}
}

func (l *RateLimiter) Inc(domain string) {
	l.checkAvanceHour()
	if _, has := l.hourCountsByDomain[l.currHour][domain]; !has {
		l.hourCountsByDomain[l.currHour][domain] = 1
	} else {
		l.hourCountsByDomain[l.currHour][domain] += 1
	}
	log.Debugf("domain %s at %d", domain, l.hourCountsByDomain[l.currHour][domain])
}

func (l *RateLimiter) checkAvanceHour() {
	hour := getHour()
	if hour != l.currHour {
		log.Debugf("avance to new hour %d", hour)
		l.currHour = hour
		l.hourCountsByDomain[hour] = make(map[string]uint)
	}
}

func (l *RateLimiter) GetCount(domain string) uint {
	l.checkAvanceHour()
	if v, has := l.hourCountsByDomain[l.currHour][domain]; has {
		return v
	}
	return 0
}
