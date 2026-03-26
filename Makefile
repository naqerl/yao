-include .env

run:
	go run main.go

dev:
	pnpx genkit-cli config set analyticsOptOut true
	pnpx genkit-cli start --non-interactive -- make run

vet:
	go fmt ./...
	go vet ./...
	staticcheck ./...
