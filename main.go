package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var MONGODB_URI string = os.Getenv("MONGODB_URI")

var mongoClient *mongo.Client

func connect_to_mongodb() {
	fmt.Println("Connecting to MongoDB...")
	fmt.Println("MONGODB_URI: len=", len(MONGODB_URI))
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(MONGODB_URI).SetServerAPIOptions(serverAPI)

	client, err := mongo.Connect(context.TODO(), opts)
	if err != nil {
		panic(err)
	}
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		panic(err)
	}
	mongoClient = client
	fmt.Println("Connected to MongoDB!")
}

func ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

func init() {
	connect_to_mongodb()
}
func main() {
	r := gin.Default()

	r.GET("/ping", ping)
	r.HEAD("/ping", ping)

	v1_tracker := r.Group("/v1")
	{
		v1_tracker.POST("/projects", v1_projects)
		v1_tracker.POST("/project/:identifier", v1_project)
		v1_tracker.POST("/project/:identifier/:client_version/:archivist/claim_task", v1_claim_task)
		v1_tracker.POST("/project/:identifier/:client_version/:archivist/update_task/:task_id", v1_update_task)
		v1_tracker.POST("/project/:identifier/:client_version/:archivist/insert_item/:item_id", v1_insert_item)
	}
	r.Run()
}
