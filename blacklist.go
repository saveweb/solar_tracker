package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
)

func withBanlist(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		archivist := c.Param("archivist")
		ip := c.RealIP()
		var archivistBan, ipBan bool

		wg := sync.WaitGroup{}

		if archivist != "" {
			wg.Go(func() {
				r := valkeyClient.Do(context.Background(), valkeyClient.B().Exists().Key("banlist:archivist:"+archivist).Build())
				if r.Error() != nil {
					log.Printf("valkey get banlist: %v", r.Error()) // passthrough on error
				} else if k, _ := r.AsInt64(); k > 0 {
					archivistBan = true
				}
			})
		}
		if ip != "" {
			wg.Go(func() {
				r := valkeyClient.Do(context.Background(), valkeyClient.B().Exists().Key("banlist:ip:"+ip).Build())
				if r.Error() != nil {
					log.Printf("valkey get banlist: %v", r.Error()) // passthrough on error
				} else if k, _ := r.AsInt64(); k > 0 {
					ipBan = true
				}
			})
		}
		wg.Wait()

		if archivistBan || ipBan {
			log.Println("Blocked request from", ip, "archivist:", archivist)
			return c.JSON(http.StatusForbidden, echo.Map{"error": "No means No!"})
		}

		return next(c)
	}
}

func admin_banlist(c echo.Context) error {
	typ := c.Param("type")
	id := c.Param("id")
	if typ != "archivist" && typ != "ip" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid type"})
	}
	if id == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid id"})
	}
	key := "banlist:" + typ + ":" + id
	r := valkeyClient.Do(context.Background(), valkeyClient.B().Set().Key(key).Value("1").Build())
	if r.Error() != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": r.Error().Error()})
	}
	return c.JSON(http.StatusOK, echo.Map{"result": "added to banlist"})
}

func admin_unbanlist(c echo.Context) error {
	typ := c.Param("type")
	id := c.Param("id")
	if typ != "archivist" && typ != "ip" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid type"})
	}
	if id == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid id"})
	}
	key := "banlist:" + typ + ":" + id
	r := valkeyClient.Do(context.Background(), valkeyClient.B().Del().Key(key).Build())
	if r.Error() != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": r.Error().Error()})
	}
	return c.JSON(http.StatusOK, echo.Map{"result": "removed from banlist"})
}

func admin_list_banlist(c echo.Context) error {
	prefix := "banlist:"
	r := valkeyClient.Do(context.Background(), valkeyClient.B().Keys().Pattern(prefix+"*").Build())
	if r.Error() != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": r.Error().Error()})
	}
	keys, err := r.AsStrSlice()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "invalid valkey response"})
	}
	banlist := strings.Builder{}
	for _, key := range keys {
		id := key[len(prefix):]
		banlist.WriteString(id)
		banlist.WriteString("\n")
	}
	return c.String(http.StatusOK, banlist.String())
}
