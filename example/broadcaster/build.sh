#! /bin/bash

CGO_ENABLED=0 GOOS=linux go build -o broadcaster ./*.go 
docker build . -t broadcaster
