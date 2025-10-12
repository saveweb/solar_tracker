package main

import (
	"context"
	"encoding/json"
	"io"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/valkey-io/valkey-go"
)

func SSEHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")
		c.Next()
	}
}

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

func stats_panel(c *gin.Context) {
	identifier := c.Param("identifier")
	project := GetProject(identifier)
	if !project.Status.Public {
		c.JSON(403, gin.H{"error": "Project is not public"})
		return
	}
	c.HTML(200, "stats.tmpl", gin.H{
		"identifier": identifier,
	})
}

func sse_stream(c *gin.Context) {
	identifier := c.Param("identifier")

	project := GetProject(identifier)
	if !project.Status.Public {
		c.JSON(403, gin.H{"error": "Project is not public"})
		return
	}

	returnsChan := make(chan valkey.PubSubMessage, 100)

	go func() {
		defer close(returnsChan)
		valkeyClient.Receive(c,
			valkeyClient.B().Subscribe().Channel("sse-"+identifier).Build(), func(msg valkey.PubSubMessage) {
				returnsChan <- msg
			})
	}()

	c.Stream(func(w io.Writer) bool {
		for msg := range returnsChan {
			c.SSEvent("message", msg.Message)
			return true
		}
		return false
	})
}
