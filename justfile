set dotenv-load := true
set dotenv-path := "{{justfile_directory()}}/.env"

# default recipe to show all recipes
default:
	@just --list

# generate code with sqlc and oapi-codegen
[group('build')]
codegen:
	go generate {{justfile_directory()}}/pkg/api
	go generate {{justfile_directory()}}/pkg/dbsqlc

# build the named services
[group('build')]
build binary: codegen
	go build -o {{justfile_directory()}}/bin/"{{binary}}" {{justfile_directory()}}/cmd/"{{binary}}"
	chmod +x {{justfile_directory()}}/bin/"{{binary}}"

[group('test')]
test:
	go test {{justfile_directory()}}/...

# keep the code tidy
[group('build')]
[group('test')]
lint: codegen
	go mod tidy
	golangci-lint run

# run locally
[group('test')]
dev binary: (build binary)
	{{justfile_directory()}}/bin/"{{binary}}"

# deploy in background
[group('deploy')]
deploy binary: (build binary)
	nohup {{justfile_directory()}}/bin/"{{binary}}" > {{justfile_directory()}}/bin/trace.log 2>&1 & echo $$! > {{justfile_directory()}}/bin/save_pid.txt

# kill the background process
[group('deploy')]
kill binary:
	kill -9 $$(cat -p "{{justfile_directory()}}"/bin/save_pid.txt) && rm {{justfile_directory()}}/bin/save_pid.txt