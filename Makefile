.PHONY: build install run test test_html

build:
	go build -o sitecap

run: build
	./sitecap --debug --http --listen localhost:9191

test:
	./sitecap --debug https://itch.io | feh -

test_html:
	echo "<html><head><title>Test</title></head><body><h1>Hello World</h1><img src='https://static.itch.io/images/app/collections@2x.png' /></body></html>" | ./sitecap --debug - | feh -

install:
	GONOPROXY=github.com/leafo/sitecap go install github.com/leafo/sitecap@latest
