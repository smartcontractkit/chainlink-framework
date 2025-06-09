package registry

//go:generate protoc -I . -I ../../.. --go_out=paths=source_relative:. feed_updated.proto
