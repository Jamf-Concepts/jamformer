.PHONY: build test lint clean

build:
	go build -o jamformer .

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -f jamformer
