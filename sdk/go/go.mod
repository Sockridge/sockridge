module github.com/Sockridge/sockridge/sdk/go

go 1.26.1

require (
	connectrpc.com/connect v1.19.1
	github.com/Sockridge/sockridge/server v0.0.0
	golang.org/x/net v0.52.0
)

require (
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/Sockridge/sockridge/server => ../../server
