// A separate module so its dependencies (the protobuf compiler) don't
// become dependencies of the service.
module github.com/Ayush1388/auctionEngine/tools/protogen

go 1.24

require (
	github.com/bufbuild/protocompile v0.14.1
	google.golang.org/protobuf v1.36.12
)

require golang.org/x/sync v0.8.0 // indirect
