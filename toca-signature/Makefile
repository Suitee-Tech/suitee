.PHONY: build run swagger

build:
	go build -o bin/server cmd/server/main.go

run:
	go run cmd/server/main.go

swagger:
	$(shell go env GOPATH)/bin/swag init --parseDependency --parseInternal -g cmd/server/main.go --dir ./,./internal
