package writetarget

//go:generate sh -c "protoc -I=. -I=../../../../.. -I=$(go list -f '{{ .Dir }}' -m github.com/smartcontractkit/capabilities/libs) --go_out=paths=source_relative:.  ./write_accepted.proto"
//go:generate sh -c "protoc -I=. -I=../../../../.. -I=$(go list -f '{{ .Dir }}' -m github.com/smartcontractkit/capabilities/libs) --go_out=paths=source_relative:.  ./write_confirmed.proto"
//go:generate sh -c "protoc -I=. -I=../../../../.. -I=$(go list -f '{{ .Dir }}' -m github.com/smartcontractkit/capabilities/libs) --go_out=paths=source_relative:.  ./write_error.proto"
//go:generate sh -c "protoc -I=. -I=../../../../.. -I=$(go list -f '{{ .Dir }}' -m github.com/smartcontractkit/capabilities/libs) --go_out=paths=source_relative:.  ./write_initiated.proto"
//go:generate sh -c "protoc -I=. -I=../../../../.. -I=$(go list -f '{{ .Dir }}' -m github.com/smartcontractkit/capabilities/libs) --go_out=paths=source_relative:.  ./write_sent.proto"
//go:generate sh -c "protoc -I=. -I=../../../../.. -I=$(go list -f '{{ .Dir }}' -m github.com/smartcontractkit/capabilities/libs) --go_out=paths=source_relative:.  ./write_skipped.proto"
