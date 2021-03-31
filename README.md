
适用与kratos-go的protobuf生成openapi3文档扩展
```golang
go get -u github.com/realotz/protoc-gen-openapi
```
生成
```bigquery
protoc --proto_path=.  --proto_path=./third_party --openapi_out=paths=source_relative:. test.proto
```