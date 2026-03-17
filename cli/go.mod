module github.com/utsav-develops/SocialAgents/cli

go 1.26.1

require (
	connectrpc.com/connect v1.19.1
	github.com/spf13/cobra v1.10.2
	github.com/utsav-develops/SocialAgents/server v0.0.0-00010101000000-000000000000
	golang.org/x/net v0.52.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace github.com/utsav-develops/SocialAgents/server => ../server
