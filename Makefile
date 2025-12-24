.PHONY: gen-ent
gen-ent: # Generate Ent codes | 生成 Ent 的代码
	go run -mod=mod entgo.io/ent/cmd/ent generate  ./ent/schema
	@echo "Generate Ent files successfully"

.PHONY: gen-kx
gen-kx: # 生成Kitex代码
	go generate ./ent
	hz model -idl=idl/system/auth_api.thrift -model_dir=api/model
	kitex idl/system/user.proto
	kitex idl/system/access.proto

.PHONY: run-kx
run-kx: # 运行Kitex
	go run services/system/user/cmd/main.go -env=false -f=etc/user.yaml
	go run services/system/setting/cmd/main.go -env=false -f=etc/setting.yaml
	go run services/system/message/cmd/main.go -env=false -f=etc/msg.yaml
	go run api/system/auth/cmd/main.go -env=false -f=etc/api/auth.yaml
	go run api/system/user/cmd/main.go -env=false -f=etc/api/user.yaml