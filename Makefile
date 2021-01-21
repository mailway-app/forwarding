VERSION = 1.0.0
DIST = $(PWD)/dist

.PHONY: clean
clean:
	rm -rf $(DIST) *.deb

$(DIST)/forwarding: rules.go smtp.go smtpd.go
	mkdir -p $(DIST)
	go build -o $(DIST)/usr/local/sbin/forwarding

.PHONY: deb
deb: $(DIST)/forwarding
	fpm -n forwarding -s dir -t deb --chdir=$(DIST) --version=$(VERSION)

.PHONY: test
test:
	go test -v ./...
