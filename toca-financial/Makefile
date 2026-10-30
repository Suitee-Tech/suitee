.PHONY: build run swagger

build:
	go build -o bin/server cmd/toca-financial/main.go

run:
	go run cmd/toca-financial/main.go

swagger:
	$(shell go env GOPATH)/bin/swag init --parseDependency -g cmd/toca-financial/main.go
