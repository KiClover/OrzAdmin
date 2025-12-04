.PHONY: gen-ent
gen-ent: # Generate Ent codes | 生成 Ent 的代码
	go run -mod=mod entgo.io/ent/cmd/ent generate --template glob="./rpc/ent/template/*.tmpl" ./rpc/ent/schema --feature $(ENT_FEATURE)
	@echo "Generate Ent files successfully"