.PHONY: build test docs

build:
	go build -o terraform-provider-opstracer .

test:
	TF_ACC_TERRAFORM_PATH=$$(which terraform) go test ./...

docs:
	go generate ./...
