build:
	go build -o sitecap


run: build
	./sitecap --http --listen localhost:9191
