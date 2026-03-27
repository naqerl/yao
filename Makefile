-include .env

run:
	go run main.go

dev:
	pnpx genkit-cli config set analyticsOptOut true
	pnpx genkit-cli start --non-interactive -- make run

sqlc:
	sqlc generate

vet:
	go fmt ./...
	go vet ./...
	staticcheck ./...

install:
	go install honnef.co/go/tools/cmd/staticcheck@0.7.0
