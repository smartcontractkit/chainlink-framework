package registry

//go:generate  sh -c "protoc -I=../../../../../../.. -I=. -I=$(go list -f '{{ .Dir }}' -m github.com/smartcontractkit/capabilities/libs) --go_out=paths=source_relative:. feed_updated.proto"
