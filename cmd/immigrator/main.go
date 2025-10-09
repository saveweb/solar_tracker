package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Meta struct {
	Identifier string `json:"identifier" bson:"identifier"`
	Slug       string `json:"slug" bson:"slug"`
	Icon       string `json:"icon" bson:"icon"`
	Deadline   string `json:"deadline" bson:"deadline"`
}

type Status struct {
	Public bool `json:"public" bson:"public"`
	Paused bool `json:"paused" bson:"paused"`
}

type Client struct {
	Version        string  `json:"version" bson:"version"`
	ClaimTaskDelay float32 `json:"claim_task_delay" bson:"claim_task_delay"`
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

const (
	defaultProjectsDir  = "data/projects"
	defaultProjectFile  = "project.json"
	dbName              = "tracker"
	collectionName      = "projects"
	mongoConnectTimeout = 10 * time.Second
	mongoWriteTimeout   = 10 * time.Second
)

func main() {
	projectsDir := flag.String("dir", defaultProjectsDir, "Path to the legacy projects directory")
	dryRun := flag.Bool("dry-run", false, "If set, only print actions without writing to MongoDB")
	flag.Parse()

	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("MONGODB_URI environment variable is required")
	}

	client, err := connectMongo(uri)
	if err != nil {
		log.Fatalf("connect to mongodb: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), mongoConnectTimeout)
		defer cancel()
		if err := client.Disconnect(ctx); err != nil {
			log.Printf("disconnect mongodb: %v", err)
		}
	}()

	collection := client.Database(dbName).Collection(collectionName)

	entries, err := os.ReadDir(*projectsDir)
	if err != nil {
		log.Fatalf("read projects dir: %v", err)
	}

	var migrated, skipped int
	for _, entry := range entries {
		if !entry.IsDir() {
			skipped++
			log.Printf("skip %s: not a directory", entry.Name())
			continue
		}

		projectPath := filepath.Join(*projectsDir, entry.Name(), defaultProjectFile)
		project, err := readProject(projectPath)
		if err != nil {
			skipped++
			log.Printf("skip %s: %v", entry.Name(), err)
			continue
		}

		if project.Meta.Identifier == "" {
			project.Meta.Identifier = entry.Name()
			log.Printf("fall back identifier for %s to directory name %s", entry.Name(), project.Meta.Identifier)
		}

		if *dryRun {
			log.Printf("[dry-run] would upsert project %s", project.Meta.Identifier)
			migrated++
			continue
		}

		if err := upsertProject(collection, project); err != nil {
			skipped++
			log.Printf("failed to upsert project %s: %v", project.Meta.Identifier, err)
			continue
		}

		migrated++
		log.Printf("upserted project %s", project.Meta.Identifier)
	}

	log.Printf("migration finished: migrated=%d skipped=%d", migrated, skipped)
}

func connectMongo(uri string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mongoConnectTimeout)
	defer cancel()

	opts := options.Client().ApplyURI(uri).SetAppName("solar-immigrator").SetCompressors([]string{"zstd", "zlib", "snappy"})
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	return client, nil
}

func readProject(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("project file %s not found", path)
		}
		return nil, fmt.Errorf("read project file %s: %w", path, err)
	}

	var project Project
	if err := json.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("unmarshal project json: %w", err)
	}
	return &project, nil
}

func upsertProject(collection *mongo.Collection, project *Project) error {
	payload, err := bson.Marshal(project)
	if err != nil {
		return fmt.Errorf("marshal project: %w", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(payload, &doc); err != nil {
		return fmt.Errorf("unmarshal project bson: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), mongoWriteTimeout)
	defer cancel()

	filter := bson.M{"meta.identifier": project.Meta.Identifier}
	update := bson.M{"$set": doc}
	opts := options.Update().SetUpsert(true)

	if _, err := collection.UpdateOne(ctx, filter, update, opts); err != nil {
		return err
	}
	return nil
}
