package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/valkey-io/valkey-go"
)

type ArchiveEvent struct {
	ProjectID string `json:"project_id"`
	Tasks     int    `json:"tasks"`
	Archived  bool   `json:"archived"` // whether the event is an archive completion
	Archivist string `json:"archivist"`
	Message   string `json:"message"`
}

var archiveEventChan = make(chan ArchiveEvent, 100)

func init() {
	go eventDispatcher()
}

func eventDispatcher() {
	for event := range archiveEventChan {
		go publishArchiveEvent(event)
	}
}

func publishArchiveEvent(event ArchiveEvent) {
	eventStr, err := json.Marshal(event)
	if err != nil {
		log.Println("Error marshalling archive event:", err)
		return
	}
	if err := valkeyClient.Do(context.Background(), valkeyClient.B().Publish().Channel("sse-"+event.ProjectID).Message(string(eventStr)).Build()).Error(); err != nil {
		log.Println("Error publishing archive event:", err)
	}
}

func stats_panel(c echo.Context) error {
	identifier := c.Param("identifier")
	project := GetProject(identifier)
	if !project.Status.Public {
		return c.JSON(http.StatusForbidden, echo.Map{"error": "Project is not public"})
	}
	return c.Render(http.StatusOK, "stats.tmpl", echo.Map{
		"identifier": identifier,
	})
}

func sse_stream(c echo.Context) error {
	identifier := c.Param("identifier")

	project := GetProject(identifier)
	if !project.Status.Public {
		return c.JSON(http.StatusForbidden, echo.Map{"error": "Project is not public"})
	}

	log.Printf("SSE client connected, ip: %v", c.RealIP())
	returnsChan := make(chan valkey.PubSubMessage, 100)

	go func() {
		defer close(returnsChan)
		valkeyClient.Receive(c.Request().Context(),
			valkeyClient.B().Subscribe().Channel("sse-"+identifier).Build(), func(msg valkey.PubSubMessage) {
				returnsChan <- msg
			})
	}()

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for msg := range returnsChan {
		select {
		case <-c.Request().Context().Done():
			return nil
		default:
		}
		if _, err := io.WriteString(w, "event: message\ndata: "+msg.Message+"\n\n"); err != nil {
			return err
		}
		w.Flush()
	}

	return nil
}
