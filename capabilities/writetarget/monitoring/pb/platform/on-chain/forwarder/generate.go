package forwarder

//go:generate protoc -I ../../.. -I=. --go_out=paths=source_relative:. report_processed.proto
