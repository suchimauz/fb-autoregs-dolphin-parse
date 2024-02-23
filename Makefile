GOOS ?= windows

.PHONY:
.SILENT:
.DEFAULT_GOAL := run

build:
	go mod download && CGO_ENABLED=0 GOOS=$(GOOS) go build -o ./.bin/app ./main.go

run: build
	docker-compose up --remove-orphans app
