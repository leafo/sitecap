.PHONY: install

build:
	go build -o sitecap


run: build
	./sitecap --debug --http --listen localhost:9191

install:
	GONOPROXY=github.com/leafo/sitecap go install github.com/leafo/sitecap@latest
