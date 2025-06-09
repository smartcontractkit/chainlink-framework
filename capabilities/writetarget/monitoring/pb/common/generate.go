// pb/common/generate.go
package common

//go:generate protoc -I=. --go_out=paths=source_relative:. execution_context.proto
