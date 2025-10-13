package main

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func ping(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{
		"message": "pong",
	})
}

func ping_mongodb(c echo.Context) error {
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
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"message":             "MongoDB is down?",
			"PrimaryEalapsedTime": PrimaryEalapsedTime.Milliseconds(),
			"NearestEalapsedTime": NearestEalapsedTime.Milliseconds(),
		})
	} else {
		return c.JSON(http.StatusOK, echo.Map{
			"message":             "MongoDB is up",
			"PrimaryEalapsedTime": PrimaryEalapsedTime.Milliseconds(),
			"NearestEalapsedTime": NearestEalapsedTime.Milliseconds(),
		})
	}
}
