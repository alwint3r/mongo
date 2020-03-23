# Mongo Storage for [OAuth 2.0](https://github.com/go-oauth2/oauth2) Using Official MongoDB Driver

This repository is based on [go-oauth2/mongo](https://github.com/go-oauth2/mongo).
The key difference is that this repository uses the official MongoDB driver instead of [globalsign/mgo](https://github.com/globalsign/mgo).

## Install

``` bash
$ go get -u -v github.com/alwint3r/mongo
```

## Usage

```go
package main

import (
	"github.com/alwint3r/mongo"
	"gopkg.in/oauth2.v3/manage"
)

func main() {
	manager := manage.NewDefaultManager()

	// use mongodb token store
	manager.MapTokenStorage(
		mongo.NewTokenStore(mongo.NewConfig(
			"mongodb://127.0.0.1:27017",
			"oauth2",
		)),
	)
	// ...
}
```

## Credits

Credits goes to [go-oauth2/mongo](https://github.com/go-oauth2/mongo) author.
