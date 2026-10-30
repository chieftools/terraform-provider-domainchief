default: fmt lint build generate

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	golangci-lint run

generate:
	cd tools && go generate ./...

fmt:
	gofmt -s -w -e .

# Runs the unit tests and the provider tests against an in-memory fake of the API.
test:
	go test -v -cover -timeout=10m ./...

# Runs the acceptance tests against the real Domain Chief API.
# Requires DOMAINCHIEF_API_TOKEN and DOMAINCHIEF_TEST_DOMAIN, see internal/provider/acceptance_test.go.
testacc:
	TF_ACC=1 go test -v -cover -timeout 30m -run TestAcc ./internal/provider/

.PHONY: default build install lint generate fmt test testacc
