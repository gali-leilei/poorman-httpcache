.PHONY: build test debug
build:
	go build -o httpcache ./cmd/httpcache
	chmod +x httpcache

test:
	go test ./...

debug:
	echo "testing"

dev: build
	set -o allexport && source .env && ./httpcache

deploy: build
	nohup ./httpcache > trace.log 2>&1 & echo $$! > save_pid.txt

sqlc:
	cd pkg/dbsqlc && sqlc generate

kill:
	kill -9 $$(cat save_pid.txt) && rm save_pid.txt