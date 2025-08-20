// pb/common/generate.go
package common

//go:generate protoc -I=. --go_out=paths=source_relative:. block_data.proto
//go:generate protoc -I=. --go_out=paths=source_relative:. execution_context_wt.proto
//go:generate protoc -I=. --go_out=paths=source_relative:. transaction_data.proto
