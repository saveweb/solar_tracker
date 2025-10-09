package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

func withBanlist(c *gin.Context) {
	archivist := c.Param("archivist")
	ip := c.ClientIP()
	var archivistBan, ipBan bool

	wg := sync.WaitGroup{}

	if archivist != "" {
		wg.Go(func() {
			r := valkeyClient.Do(context.Background(), valkeyClient.B().Get().Key("banlist:archivist:"+archivist).Build())
			if r.Error() != nil {
				log.Printf("valkey get banlist: %v", r.Error()) // passthrough on error
			} else {
				archivistBan = true
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "NO means NO! (A)"})
			}
		})
	}
	if ip != "" {
		wg.Go(func() {
			r := valkeyClient.Do(context.Background(), valkeyClient.B().Get().Key("banlist:ip:"+ip).Build())
			if r.Error() != nil {
				log.Printf("valkey get banlist: %v", r.Error()) // passthrough on error
			} else {
				ipBan = true
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "NO means NO! (I)"})
			}
		})
	}
	wg.Wait()

	if archivistBan || ipBan {
		return
	}

	c.Next()
}

func admin_banlist(c *gin.Context) {
	typ := c.Param("type")
	id := c.Param("id")
	if typ != "archivist" && typ != "ip" {
		c.JSON(400, gin.H{"error": "invalid type"})
		return
	}
	if id == "" {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	key := "banlist:" + typ + ":" + id
	r := valkeyClient.Do(context.Background(), valkeyClient.B().Set().Key(key).Value("1").Build())
	if r.Error() != nil {
		c.JSON(500, gin.H{"error": r.Error().Error()})
		return
	}
	c.JSON(200, gin.H{"result": "added to banlist"})
}

func admin_unbanlist(c *gin.Context) {
	typ := c.Param("type")
	id := c.Param("id")
	if typ != "archivist" && typ != "ip" {
		c.JSON(400, gin.H{"error": "invalid type"})
		return
	}
	if id == "" {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	key := "banlist:" + typ + ":" + id
	r := valkeyClient.Do(context.Background(), valkeyClient.B().Del().Key(key).Build())
	if r.Error() != nil {
		c.JSON(500, gin.H{"error": r.Error().Error()})
		return
	}
	c.JSON(200, gin.H{"result": "removed from banlist"})
}

func admin_list_banlist(c *gin.Context) {
	prefix := "banlist:"
	r := valkeyClient.Do(context.Background(), valkeyClient.B().Keys().Pattern(prefix+"*").Build())
	if r.Error() != nil {
		c.JSON(500, gin.H{"error": r.Error().Error()})
		return
	}
	keys, err := r.AsStrSlice()
	if err != nil {
		c.JSON(500, gin.H{"error": "invalid valkey response"})
		return
	}
	banlist := strings.Builder{}
	for _, key := range keys {
		id := key[len(prefix):]
		banlist.WriteString(id)
		banlist.WriteString("\n")
	}
	c.String(http.StatusOK, banlist.String())
}
