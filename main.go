package main

import (
	"embed"
	_ "embed"
	"html/template"
	"io"
	"log"
	"os"
	"runtime"
	"sync"

	"github.com/grafana/pyroscope-go"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

//go:embed templates/*
var embedFS embed.FS

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func startProfiler() (*pyroscope.Profiler, error) {
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)
	return pyroscope.Start(pyroscope.Config{
		ApplicationName: "solar_tracker",
		ServerAddress:   "https://service-" + "pyroscope" + ".saveweb.org/",
		Logger:          pyroscope.StandardLogger,
		Tags:            map[string]string{"hostname": os.Getenv("HOSTNAME")},
		ProfileTypes: []pyroscope.ProfileType{
			// these profile types are enabled by default:
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			// these profile types are optional:
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
}

func main() {
	profiler, err := startProfiler()
	if err != nil {
		log.Fatalf("failed to start profiler: %v", err)
	}
	defer profiler.Stop()

	e := echo.New()
	t := &Template{
		templates: template.Must(template.New("").ParseFS(embedFS, "templates/*")),
	}
	e.Renderer = t
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format:           "${time_custom} |${status}| ${remote_ip} | ${method} \"${uri}\" ${latency_human} e=${error}, in=${bytes_in} out=${bytes_out}\n",
		CustomTimeFormat: "2006-01-02 15:04:05",
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.Decompress())

	e.GET("/ping", ping)
	e.HEAD("/ping", ping)

	e.GET("/ping_mongodb", ping_mongodb)
	e.HEAD("/ping_mongodb", ping_mongodb)

	v1_tracker := e.Group("/v1")
	v1_tracker_with_banlist := e.Group("/v1", withBanlist)
	{
		v1_tracker.POST("/projects", v1_projects)
		v1_tracker.POST("/project/:identifier", v1_project)
		v1_tracker_with_banlist.POST("/project/:identifier/:client_version/:archivist/claim_task", v1_claim_task)
		v1_tracker_with_banlist.POST("/project/:identifier/:client_version/:archivist/update_task/:task_id", v1_update_task)
		v1_tracker_with_banlist.POST("/project/:identifier/:client_version/:archivist/insert_item/:item_id", v1_insert_item)
		v1_tracker_with_banlist.POST("/project/:identifier/:client_version/:archivist/insert_many/:size", v1_insert_many) // :size unused
	}

	admin := e.Group("/admin", middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
		if username == "admin" && password == ADMIN_PASSWORD {
			return true, nil
		}
		return false, nil
	}))
	adminApi := admin.Group("/api")
	{
		adminApi.GET("/banlist", admin_list_banlist)
		adminApi.POST("/banlist/:type/:id", admin_banlist)
		adminApi.DELETE("/banlist/:type/:id", admin_unbanlist)
	}

	panel := e.Group("/stats")
	{
		panel.GET("/live/:identifier", stats_panel)
		panel.GET("/live/:identifier/sse", sse_stream)
	}

	e.Logger.Fatal(e.Start(":8080"))
}
