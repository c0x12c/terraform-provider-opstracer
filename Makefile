.PHONY: build test docs version

build:
	go build -o terraform-provider-opstracer .

test:
	TF_ACC_TERRAFORM_PATH=$$(which terraform) go test ./...

docs:
	go generate ./...

MINOR = $(word 1,$(subst ., ,$(VERSION))).$(word 2,$(subst ., ,$(VERSION)))

# Run before tagging: points the version constraint in examples, README and docs at VERSION's minor line.
version:
	@test -n "$(VERSION)" || { echo "usage: make version VERSION=0.2.1"; exit 1; }
	sed -i.bak -E 's/version = "~> [0-9.]+"/version = "~> $(MINOR).0"/' examples/provider/provider.tf README.md
	rm -f examples/provider/provider.tf.bak README.md.bak
	go generate ./...
