GOHOSTOS:=$(shell go env GOHOSTOS)

ifeq ($(GOHOSTOS), windows)
	# Windows环境下使用Git Bash执行find命令
	Git_Bash=$(subst \,/,$(subst cmd\,bin\bash.exe,$(dir $(shell where git))))
	API_PROTO_FILES=$(shell $(Git_Bash) -c "find ./api -name *.proto")
else
	API_PROTO_FILES=$(shell find ./api -name *.proto)
endif

.PHONY: api
# generate api proto
api:
	protoc --proto_path=./api \
 	       --go_out=paths=source_relative:./api \
	       $(API_PROTO_FILES)
