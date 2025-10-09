package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type Meta struct {
	Identifier string `json:"identifier" bson:"identifier"`
	Slug       string `json:"slug" bson:"slug"`         // 一句话项目说明
	Icon       string `json:"icon" bson:"icon"`         // 图标 URL
	Deadline   string `json:"deadline" bson:"deadline"` // 截止日期，没有格式要求
}

type Status struct {
	Public bool `json:"public" bson:"public"` // 是否公开。（不在 /projects 列表中列出，但 /project 可以请求）
	Paused bool `json:"paused" bson:"paused"` // 是否暂停。暂停后不再接受新的 claim_task 请求。但是仍可 update_task 和 insert_item
}

type Client struct {
	Version        string  `json:"version" bson:"version"`                   // 推荐 "大版本.小版本"。版本不对会拒绝各种请求。
	ClaimTaskDelay float32 `json:"claim_task_delay" bson:"claim_task_delay"` // claim_task 之后多久才能再次 claim_task
}

type Mongodb struct {
	DbName          string `json:"db_name" bson:"db_name"`
	ItemCollection  string `json:"item_collection" bson:"item_collection"`
	QueueCollection string `json:"queue_collection" bson:"queue_collection"`
}

type Project struct {
	Meta    Meta    `json:"meta" bson:"meta"`
	Status  Status  `json:"status" bson:"status"`
	Client  Client  `json:"client" bson:"client"`
	Mongodb Mongodb `json:"mongodb" bson:"mongodb"`
}

const projectsDatabaseName = "tracker"
const projectsCollectionName = "projects"

var projectQueryTimeout = 5 * time.Second

func GetProjectsName() []string {
	ctx, cancel := context.WithTimeout(context.Background(), projectQueryTimeout)
	defer cancel()

	collection := mongoClient.Database(projectsDatabaseName).Collection(projectsCollectionName)
	identifiers, err := collection.Distinct(ctx, "meta.identifier", bson.M{})
	if err != nil {
		panic(err)
	}

	projects := []string{}
	for _, id := range identifiers {
		if identifier, ok := id.(string); ok && identifier != "" {
			projects = append(projects, identifier)
		}
	}
	return projects
}

var projectCache = ttlcache.New(
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
	ctx, cancel := context.WithTimeout(context.Background(), projectQueryTimeout)
	defer cancel()

	collection := mongoClient.Database(projectsDatabaseName).Collection(projectsCollectionName)
	project := Project{}
	err := collection.FindOne(ctx, bson.M{"meta.identifier": project_name}).Decode(&project)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		panic(err)
	}

	projectCache.Set(project_name, &project, ttlcache.DefaultTTL)
	return &project
}

func GetProjects() []Project {
	ctx, cancel := context.WithTimeout(context.Background(), projectQueryTimeout)
	defer cancel()

	collection := mongoClient.Database(projectsDatabaseName).Collection(projectsCollectionName)
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		panic(err)
	}
	defer cursor.Close(ctx)

	projects := []Project{}
	for cursor.Next(ctx) {
		project := Project{}
		if err := cursor.Decode(&project); err != nil {
			panic(err)
		}
		projects = append(projects, project)
	}
	if err := cursor.Err(); err != nil {
		panic(err)
	}
	return projects
}
