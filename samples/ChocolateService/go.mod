module authway-samples/ChocolateService

go 1.21

replace authway-samples/shared => ../shared

require authway-samples/shared v0.0.0-00010101000000-000000000000

require (
	github.com/golang/protobuf v1.5.3 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/oauth2 v0.15.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)
