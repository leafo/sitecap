.PHONY: build install run test test_html test_mcp release

BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
COMMIT_HASH := $(shell git rev-parse --short HEAD)
LDFLAGS := -ldflags "-X main.buildDate=$(BUILD_DATE) -X main.commitHash=$(COMMIT_HASH)"

build:
	go build $(LDFLAGS) -o sitecap

run: build
	./sitecap --debug --http --listen localhost:9191

test:
	./sitecap --debug https://itch.io | feh -

test_html:
	echo "<html><head><title>Test</title></head><body><h1>Hello World</h1><img src='https://static.itch.io/images/app/collections@2x.png' /></body></html>" | ./sitecap --debug - | feh -


test_mcp: build
	npx @modelcontextprotocol/inspector ./sitecap  -mcp

install:
	go install $(LDFLAGS) .

release:
	gh release create "$$(date +%Y-%m-%d)" --generate-notes --title "Release $$(date +%Y-%m-%d)"
