package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

type Meta struct {
	Identifier string `json:"identifier"`
	Slug       string `json:"slug"`     // 一句话项目说明
	Icon       string `json:"icon"`     // 图标 URL
	Deadline   string `json:"deadline"` // 截止日期，没有格式要求
}

type Status struct {
	Public bool `json:"public"` // 是否公开。（不在 /projects 列表中列出，但 /project 可以请求）
	Paused bool `json:"paused"` // 是否暂停。暂停后不再接受新的 claim_task 请求。但是仍可 update_task 和 insert_item
}

type Client struct {
	Version        string  `json:"version"`          // 推荐 "大版本.小版本"。版本不对会拒绝各种请求。
	ClaimTaskDelay float32 `json:"claim_task_delay"` // claim_task 之后多久才能再次 claim_task
}

type Mongodb struct {
	DbName          string `json:"db_name"`
	ItemCollection  string `json:"item_collection"`
	QueueCollection string `json:"queue_collection"`
	CustomDocIDName string `json:"custom_doc_id_name"` // CustomIDName 目前只是为了兼容 lowapk_v2 存档项目而留的字段。它用的 feed_id 而不是 id
	// 计划未来等转换好数据库 scheme 之后就弃用。
}

type Project struct {
	Meta    Meta    `json:"meta"`
	Status  Status  `json:"status"`
	Client  Client  `json:"client"`
	Mongodb Mongodb `json:"mongodb"`
}

const PROJECTS_DIR = "data/projects"
const PROJECT_FILE = "project.json"

func GetProjectsName() []string {
	projects := []string{}
	// {projects_dir}/{project}/project.json

	dirs, _ := os.ReadDir(PROJECTS_DIR)
	for _, dir := range dirs {
		if dir.IsDir() {
			projects = append(projects, dir.Name())
		} else {
			fmt.Printf("%s is not a directory, skipping...\n", dir.Name())
			continue
		}
	}
	return projects
}

var projectCache = ttlcache.New[string, *Project](
	ttlcache.WithTTL[string, *Project](5*time.Second),
	ttlcache.WithDisableTouchOnHit[string, *Project](),
)

func init() {
	fmt.Println("init project cache...")
	go projectCache.Start()
	fmt.Println("init project cache, done.")
}

func GetProject(project_name string) *Project {
	item := projectCache.Get(project_name)
	if item != nil {
		return item.Value()
	}
	// project_file_path := fmt.Sprintf("%s/%s/%s", PROJECTS_DIR, project_name, PROJECT_FILE)
	project_file_path_safe, err := filepath.Abs(filepath.Join(PROJECTS_DIR, project_name, PROJECT_FILE))
	if err != nil {
		return nil
	}
	project_data, _ := os.ReadFile(project_file_path_safe)
	project := Project{}

	err = json.Unmarshal(project_data, &project)
	if err != nil {
		return nil // return nil if project not found
	}

	projectCache.Set(project_name, &project, ttlcache.DefaultTTL)
	return &project
}

func GetProjects() []Project {
	projects := []Project{}
	// {projects_dir}/{project}/project.json

	dirs, err := os.ReadDir(PROJECTS_DIR)
	if err != nil {
		panic(err)
	}
	for _, dir := range dirs {
		if dir.IsDir() {
			project_file_path := fmt.Sprintf("%s/%s/%s", PROJECTS_DIR, dir.Name(), PROJECT_FILE)
			project_data, _ := os.ReadFile(project_file_path)

			project := Project{}
			err = json.Unmarshal(project_data, &project)
			if err != nil {
				panic(err)
			}
			projects = append(projects, project)
		} else {
			fmt.Printf("%s is not a directory, skipping...\n", dir.Name())
			continue
		}
	}
	return projects
}
