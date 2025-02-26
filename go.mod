module github.com/pricempire/cs2-inspect-service-go

go 1.23.3

require (
	github.com/Philipp15b/go-steam/v3 v3.0.0
	github.com/joho/godotenv v1.5.1
	google.golang.org/protobuf v1.30.0
)

require golang.org/x/net v0.9.0

replace github.com/Philipp15b/go-steam/v3 => ./go-steam
