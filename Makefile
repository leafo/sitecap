build:
	go build -o sitecap


run: build
	./sitecap --debug --http --listen localhost:9191
