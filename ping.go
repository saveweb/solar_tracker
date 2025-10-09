package main

import (
	"time"

	"github.com/gin-gonic/gin"
)

func ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
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
