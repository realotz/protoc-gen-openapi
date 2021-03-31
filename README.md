
适用与kratos-go的protobuf生成openapi3文档扩展
ps 暂不支持map结构，map定义会返回一个空的object
```golang
go get -u github.com/realotz/protoc-gen-openapi
```
生成
```bigquery
protoc --proto_path=.  --proto_path=./third_party --openapi_out=paths=source_relative:. test.proto
```