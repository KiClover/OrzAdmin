.PHONY: gen-ent
gen-ent: # Generate Ent codes | 生成 Ent 的代码
	go run -mod=mod entgo.io/ent/cmd/ent generate  ./ent/schema
	@echo "Generate Ent files successfully"

.PHONY: run-kx-user
run-kx-user: # Generate proto codes | 生成 proto 的代码
	go generate ./ent
	go run services/system/user/cmd/main.go -env=false -f=etc/user.yaml
	go run api/system/auth/cmd/main.go -env=false -f=etc/auth.yaml
	hz model -idl=idl/system/auth_api.thrift -model_dir=api/model
	kitex idl/system/user.proto