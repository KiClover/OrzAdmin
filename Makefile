.DEFAULT_GOAL := help
SHELL := sh

GO ?= go
KITEX ?= kitex
HZ ?= hz
SCOPE ?= system

# make treats positional arguments as goals. Consume only the IDL argument of
# kx/hz, so typos in other commands still produce a normal "No rule" error.
ifneq ($(filter kx hz,$(MAKECMDGOALS)),)
ifneq ($(words $(MAKECMDGOALS)),2)
$(error Usage: make kx <file.proto> OR make hz <file.thrift>)
endif
IDL := $(word 2,$(MAKECMDGOALS))
ifeq ($(firstword $(MAKECMDGOALS)),kx)
ifneq ($(suffix $(IDL)),.proto)
$(error Usage: make kx <file.proto>)
endif
else ifeq ($(firstword $(MAKECMDGOALS)),hz)
ifneq ($(suffix $(IDL)),.thrift)
$(error Usage: make hz <file.thrift>)
endif
else
$(error Put the command first: make kx <file.proto> OR make hz <file.thrift>)
endif
ifeq ($(wildcard idl/$(SCOPE)/$(IDL)),)
$(error IDL file not found: idl/$(SCOPE)/$(IDL))
endif
.PHONY: $(IDL)
$(IDL):
	@:
endif

.PHONY: help ent gen-ent generate-ent kx hz
.PHONY: run-user run-setting run-message run-access
.PHONY: run-api-auth run-api-user run-api-permission

help: ## Show available commands
	@printf '%s\n' \
	  'Code generation:' \
	  '  make ent                     Generate Ent code from ent/schema' \
	  '  make generate-ent            Run go generate ./ent (custom templates)' \
	  '  make kx user.proto           Run kitex idl/system/user.proto' \
	  '  make hz user_api.thrift      Generate Hertz models in api/model' \
	  '  Replace the IDL filename with another file under idl/system/.' \
	  '  Use SCOPE=<dir> for other domains: make kx bill.proto SCOPE=finance' \
	  '' \
	  'RPC services (one terminal per service):' \
	  '  make run-user                Use etc/user.yaml' \
	  '  make run-setting             Use etc/setting.yaml' \
	  '  make run-message             Use etc/msg.yaml' \
	  '  make run-access              Use etc/access.yaml' \
	  '' \
	  'HTTP API services (one terminal per service):' \
	  '  make run-api-auth            Use etc/api/auth.yaml' \
	  '  make run-api-user            Use etc/api/user.yaml' \
	  '  make run-api-permission      Use etc/api/permission.yaml' \
	  '' \
	  'Tools can be overridden: make kx user.proto KITEX=/path/to/kitex'

# Keep the original Ent command and its old target name.
ent: ## Generate Ent code
	$(GO) run -mod=mod entgo.io/ent/cmd/ent generate ./ent/schema

gen-ent: ent ## Alias for ent

generate-ent: ## Generate Ent code using ent/generate.go
	$(GO) generate ./ent

kx: ## Generate one Kitex IDL: make kx user.proto [SCOPE=system]
	$(KITEX) idl/$(SCOPE)/$(IDL)

hz: ## Generate one Hertz model: make hz user_api.thrift [SCOPE=system]
	$(HZ) model -idl=idl/$(SCOPE)/$(IDL) -model_dir=api/model

run-user: ## Run the user RPC service
	$(GO) run services/system/user/cmd/main.go -env=false -f=etc/user.yaml

run-setting: ## Run the setting RPC service
	$(GO) run services/system/setting/cmd/main.go -env=false -f=etc/setting.yaml

run-message: ## Run the message RPC service
	$(GO) run services/system/message/cmd/main.go -env=false -f=etc/msg.yaml

run-access: ## Run the access RPC service
	$(GO) run services/system/access/cmd/main.go -env=false -f=etc/access.yaml

run-api-auth: ## Run the auth HTTP API
	$(GO) run api/system/auth/cmd/main.go -env=false -f=etc/api/auth.yaml

run-api-user: ## Run the user HTTP API
	$(GO) run api/system/user/cmd/main.go -env=false -f=etc/api/user.yaml

run-api-permission: ## Run the permission HTTP API
	$(GO) run api/system/permission/cmd/main.go -env=false -f=etc/api/permission.yaml
