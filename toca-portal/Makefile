.PHONY: build run swagger

build:
	go build -o bin/server cmd/toca-portal/main.go

run:
	go run cmd/toca-portal/main.go

swagger:
	$(shell go env GOPATH)/bin/swag init --parseDependency --parseInternal -g cmd/toca-portal/main.go --dir ./,./internal
