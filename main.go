package main

import (
	"os"
	"sync"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

var MONGODB_URI string = os.Getenv("MONGODB_URI")
var valkeyInitAddress = []string{os.Getenv("VALKEY_INIT_ADDRESS")}
var ADMIN_PASSWORD string = os.Getenv("ADMIN_PASSWORD")

func init() {
	if ADMIN_PASSWORD == "" {
		panic("ADMIN_PASSWORD is not set")
	}

	wg := sync.WaitGroup{}
	wg.Go(connect_to_mongodb)
	wg.Go(connect_to_valkey)
	wg.Wait()
}

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Use(gzip.Gzip(gzip.NoCompression, gzip.WithDecompressOnly(), gzip.WithDecompressFn(gzip.DefaultDecompressHandle)))

	r.GET("/ping", ping)
	r.HEAD("/ping", ping)

	r.GET("/ping_mongodb", ping_mongodb)
	r.HEAD("/ping_mongodb", ping_mongodb)

	v1_tracker := r.Group("/v1")
	v1_tracker_with_banlist := r.Group("/v1", withBanlist)
	{
		v1_tracker.POST("/projects", v1_projects)
		v1_tracker.POST("/project/:identifier", v1_project)
		v1_tracker_with_banlist.POST("/project/:identifier/:client_version/:archivist/claim_task", v1_claim_task)
		v1_tracker_with_banlist.POST("/project/:identifier/:client_version/:archivist/update_task/:task_id", v1_update_task)
		v1_tracker_with_banlist.POST("/project/:identifier/:client_version/:archivist/insert_item/:item_id", v1_insert_item)
		v1_tracker_with_banlist.POST("/project/:identifier/:client_version/:archivist/insert_many/:size", v1_insert_many) // :size unused
	}

	admin := r.Group("/admin", gin.BasicAuth(gin.Accounts{
		"admin": ADMIN_PASSWORD,
	}))
	adminApi := admin.Group("/api")
	{
		adminApi.GET("/banlist", admin_list_banlist)
		adminApi.POST("/banlist/:type/:id", admin_banlist)
		adminApi.DELETE("/banlist/:type/:id", admin_unbanlist)
	}

	panel := r.Group("/stats")
	{
		panel.GET("/live/:identifier", stats_panel)
		panel.GET("/live/:identifier/sse", SSEHeadersMiddleware(), sse_stream)
	}

	r.Run()
}
