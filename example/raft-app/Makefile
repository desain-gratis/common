bundle:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o broadcaster ./*.go 
	docker build . -t broadcaster --platform linux/amd64
	docker save -o broadcaster.tar broadcaster:latest

build:
	CGO_ENABLED=0 GOOS=linux go build -o broadcaster ./*.go 
	docker build . -t broadcaster
run:
	docker compose up
