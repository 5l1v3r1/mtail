# Copyright 2011 Google Inc. All Rights Reserved.
# This file is available under the Apache license.

# Set the timeout for tests run under the race detector.
timeout := 60s
ifeq ($(TRAVIS),true)
timeout := 5m
endif
ifeq ($(CIRCLECI),true)
timeout := 5m
endif

GOFILES=$(shell find . -name '*.go' -a ! -name '*_test.go')

GOTESTFILES=$(shell find . -name '*_test.go')

CLEANFILES+=\
	vm/parser.go\
	vm/y.output\


all: mtail

.PHONY: clean covclean crossclean
clean: covclean crossclean
	rm -f $(CLEANFILES) .*dep-stamp
covclean:
	rm -f *.coverprofile coverage.html
crossclean:
	rm -rf build

version := $(shell git describe --tags)
revision := $(shell git rev-parse HEAD)
release := $(shell git describe --tags | cut -d"-" -f 1,2)

GO_LDFLAGS := "-X main.Version=${version} -X main.Revision=${revision}"

install mtail: $(GOFILES) .dep-stamp
	go install -ldflags $(GO_LDFLAGS)

vm/parser.go: vm/parser.y .gen-dep-stamp
	go generate -x ./vm

emgen/emgen: emgen/emgen.go
	cd emgen && go build

.PHONY: install_crossbuild
install_crossbuild: .crossbuild-dep-stamp

.crossbuild-dep-stamp:
	go get github.com/mitchellh/gox
	touch $@

GOX_OSARCH ?= "linux/amd64 windows/amd64 darwin/amd64"
#GOX_OSARCH := ""

.PHONY: crossbuild
crossbuild: install_crossbuild $(GOFILES) .dep-stamp
	mkdir -p build
	gox --output="./build/mtail_${release}_{{.OS}}_{{.Arch}}" -osarch=$(GOX_OSARCH) -ldflags $(GO_LDFLAGS)

.PHONY: test check
check test: $(GOFILES) $(GOTESTFILES)
	go test -timeout 10s ./...

.PHONY: testrace
testrace: $(GOFILES) $(GOTESTFILES)
	go test -timeout ${timeout} -race -v ./...

.PHONY: smoke
smoke: $(GOFILES) $(GOTESTFILES)
	go test -timeout 1s -test.short ./...

.PHONY: ex_test
ex_test: ex_test.go testdata/* examples/*
	go test -run TestExamplePrograms --logtostderr

.PHONY: bench
bench: $(GOFILES) $(GOTESTFILES) .dep-stamp
	go test -bench=. -timeout=60s -run=XXX ./...

.PHONY: bench_cpu
bench_cpu:
	go test -bench=. -run=XXX -timeout=60s -cpuprofile=cpu.out
.PHONY: bench_mem
bench_mem:
	go test -bench=. -run=XXX -timeout=60s -memprofile=mem.out

.PHONY: recbench
recbench: $(GOFILES) $(GOTESTFILES) .dep-stamp
	go test -bench=. -run=XXX --record_benchmark ./...

.PHONY: regtest
regtest:
	tests/regtest.sh

PACKAGES := $(shell find . -name '*.go' -exec dirname {} \; | sort -u)

PHONY: coverage
coverage: gover.coverprofile
gover.coverprofile: $(GOFILES) $(GOTESTFILES) .dep-stamp
	for package in $(PACKAGES); do\
		go test -covermode=count -coverprofile=$$(echo $$package | tr './' '__').coverprofile ./$$package;\
    done
	gover

.PHONY: covrep
covrep: coverage.html
	xdg-open $<
coverage.html: gover.coverprofile
	go tool cover -html=$< -o $@

.PHONY: testall
testall: testrace bench regtest

.PHONY: install_deps
install_deps: .dep-stamp

IMPORTS := $(shell go list -f '{{join .Imports "\n"}}' ./... | sort | uniq | grep -v mtail)
TESTIMPORTS := $(shell go list -f '{{join .TestImports "\n"}}' ./... | sort | uniq | grep -v mtail)

.dep-stamp: vm/parser.go
	@echo "Install all dependencies, ensuring they're updated"
	go get -u -v $(IMPORTS)
	go get -u -v $(TESTIMPORTS)
	touch $@

.PHONY: install_gen_deps
install_gen_deps: .gen-dep-stamp

.gen-dep-stamp:
	go get golang.org/x/tools/cmd/goyacc
	touch $@

.PHONY: install_coverage_deps
install_coverage_deps: .cov-dep-stamp vm/parser.go

.cov-dep-stamp: install_deps
	go get golang.org/x/tools/cmd/cover
	go get github.com/sozorogami/gover
	go get github.com/mattn/goveralls
	touch $@

ifeq ($(CIRCLECI),true)
  COVERALLS_SERVICE := circle-ci
endif
ifeq ($(TRAVIS),true)
  COVERALLS_SERVICE := travis-ci
endif

upload_to_coveralls: gover.coverprofile
	goveralls -coverprofile=gover.coverprofile -service=$(COVERALLS_SERVICE)

# Append the bin subdirs of every element of the GOPATH list to PATH, so we can find goyacc.
empty :=
space := $(empty) $(empty)
export PATH := $(PATH):$(subst $(space),:,$(patsubst %,%/bin,$(subst :, ,$(GOPATH))))
