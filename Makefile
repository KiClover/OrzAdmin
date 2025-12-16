.PHONY: gen-ent
gen-ent: # Generate Ent codes | 生成 Ent 的代码
	go run -mod=mod entgo.io/ent/cmd/ent generate  ./ent/schema
	@echo "Generate Ent files successfully"

.PHONY: run-kx
run-kx: # 运行Kitex
	go generate ./ent
	go run services/system/user/cmd/main.go -env=false -f=etc/user.yaml
	go run services/system/setting/cmd/main.go -env=false -f=etc/setting.yaml
	go run services/system/message/cmd/main.go -env=false -f=etc/message.yaml
	go run api/system/auth/cmd/main.go -env=false -f=etc/auth.yaml
	hz model -idl=idl/system/auth_api.thrift -model_dir=api/model
	kitex idl/system/user.proto