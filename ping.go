package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
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
func ping_mongodb(c *gin.Context) {
	primaryChan := make(chan time.Duration)
	nearestChan := make(chan time.Duration)
	errChan := make(chan error)

	go func() {
		PrimaryEalapsedTime, PrimaryErr := ping_mongodb_primary()
		primaryChan <- PrimaryEalapsedTime
		errChan <- PrimaryErr
	}()

	go func() {
		NearestEalapsedTime, NearestErr := ping_mongodb_nearest()
		nearestChan <- NearestEalapsedTime
		errChan <- NearestErr
	}()

	PrimaryEalapsedTime := <-primaryChan
	NearestEalapsedTime := <-nearestChan
	PrimaryErr := <-errChan
	NearestErr := <-errChan

	if PrimaryErr != nil || NearestErr != nil {
		c.JSON(500, gin.H{
			"message":             "MongoDB is down?",
			"PrimaryEalapsedTime": PrimaryEalapsedTime.Milliseconds(),
			"NearestEalapsedTime": NearestEalapsedTime.Milliseconds(),
		})
	} else {
		c.JSON(200, gin.H{
			"message":             "MongoDB is up",
			"PrimaryEalapsedTime": PrimaryEalapsedTime.Milliseconds(),
			"NearestEalapsedTime": NearestEalapsedTime.Milliseconds(),
		})
	}
}
