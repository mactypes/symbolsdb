
symbols.zip: symbols
	zip -r symbols.zip ./symbols

symbols: cache/meta cache/docSet.db
	go run load.go
	go run inflate.go

cache/meta: cache/docSet.db
	go run load.go
	go run fetch.go