.PHONY: build test
build:
	go build -o bin/ddil ./cmd/ddil
test:
	go vet ./... && go test -race -v -run . ./... 2>&1 | grep -E "^(=== RUN|--- |ok|FAIL|    ddil)"
