.PHONY: build test debug codegen

# generate code with sqlc and oapi-codegen
codegen:
	go generate ./pkg/api
	go generate ./pkg/dbsqlc

# build two services
build: codegen
	go build -o internalcache ./cmd/internalcache
	go build -o httpcache ./cmd/httpcache
	chmod +x internalcache
	chmod +x httpcache

test:
	go test ./...

dev: build
	set -o allexport && source .env && ./internalcache

deploy: build
	nohup ./httpcache > trace.log 2>&1 & echo $$! > save_pid.txt

kill:
	kill -9 $$(cat save_pid.txt) && rm save_pid.txt