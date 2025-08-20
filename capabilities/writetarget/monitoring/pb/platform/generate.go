package writetarget

//go:generate protoc -I=. -I=../../../../.. --go_out=paths=source_relative:.  ./write_accepted.proto
//go:generate protoc -I=. -I=../../../../.. --go_out=paths=source_relative:.  ./write_confirmed.proto
//go:generate protoc -I=. -I=../../../../.. --go_out=paths=source_relative:.  ./write_error.proto
//go:generate protoc -I=. -I=../../../../.. --go_out=paths=source_relative:.  ./write_initiated.proto
//go:generate protoc -I=. -I=../../../../.. --go_out=paths=source_relative:.  ./write_sent.proto
//go:generate protoc -I=. -I=../../../../.. --go_out=paths=source_relative:.  ./write_skipped.proto
