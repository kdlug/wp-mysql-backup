.PHONY: install build

SERVICE=wp-mysql-backup

install:
	# go get -u github.com/golang/dep/cmd/dep
	dep ensure -vendor-only

build:  install
	go build -o ${SERVICE}

pack:
	# build docker image
	GOOS=linux make build
	docker build -t kdlug/${SERVICE}:latest .

clean:
	# remove binary file
	rm ./${SERVICE}

run:
	docker-compose run --rm wp-mysql-backup 

debug:
	docker-compose run --rm --entrypoint=/bin/sh wp-mysql-backup
