package main

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var mongoClient *mongo.Client

func connect_to_mongodb() {
	log.Println("Connecting to MongoDB...")
	log.Println("MONGODB_URI: len=", len(MONGODB_URI))
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(MONGODB_URI).SetServerAPIOptions(serverAPI).SetAppName("solar-tracker").SetCompressors([]string{"zstd", "zlib", "snappy"})
	log.Println("AppName: ", *opts.AppName)
	log.Println("Compressors: ", opts.Compressors)

	client, err := mongo.Connect(context.TODO(), opts)
	if err != nil {
		panic(err)
	}
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		panic(err)
	}
	mongoClient = client
	log.Println("Connected to MongoDB!")
}

func ping_mongodb_primary() (time.Duration, error) {
	PrimaryStartTime := time.Now()
	// ping primary
	PrimaryErr := mongoClient.Ping(context.Background(), readpref.Primary())
	PrimaryEalapsedTime := time.Since(PrimaryStartTime)

	return PrimaryEalapsedTime, PrimaryErr
}

func ping_mongodb_nearest() (time.Duration, error) {
	NearestStartTime := time.Now()
	// ping nearest
	NearestErr := mongoClient.Ping(context.Background(), readpref.Nearest())
	NearestEalapsedTime := time.Since(NearestStartTime)

	return NearestEalapsedTime, NearestErr
}
