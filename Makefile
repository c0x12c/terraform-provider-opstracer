.PHONY: build test docs version

build:
	go build -o terraform-provider-opstracer .

test:
	TF_ACC_TERRAFORM_PATH=$$(which terraform) go test ./...

docs:
	go generate ./...

# Writes VERSION into the version constraint shown in examples, README and docs/. The Release workflow runs it.
version:
	@test -n "$(VERSION)" || { echo "usage: make version VERSION=0.2.1"; exit 1; }
	sed -i.bak -E 's/version = "~> [0-9.]+"/version = "~> $(VERSION)"/' examples/provider/provider.tf README.md
	rm -f examples/provider/provider.tf.bak README.md.bak
	go generate ./...
