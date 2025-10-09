package main

import (
	"log"

	"github.com/valkey-io/valkey-go"
)

var valkeyClient valkey.Client

func connect_to_valkey() {
	log.Println("Connecting to Valkey...")
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: valkeyInitAddress, SelectDB: 0})
	if err != nil {
		panic(err)
	}
	valkeyClient = client
	log.Println("Connected to Valkey!")
}
