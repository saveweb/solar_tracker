package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const DEFAULT_DOC_ID_NAME = "id"

func v1_project(c *gin.Context) {
	identifier := c.Param("identifier")
	if !is_safe_sting(identifier) {
		c.JSON(400, gin.H{"error": "Invalid identifier"})
		return
	}
	project := GetProject(identifier)
	if project == nil {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
		return
	}
	c.JSON(200, project)
}

func v1_projects(c *gin.Context) {
	projects := GetProjects()
	pub_projects := []Project{}
	show_private := c.Query("show_private")
	if show_private != "" {
		c.JSON(200, projects)
		return
	}

	for _, project := range projects {
		if project.Status.Public {
			pub_projects = append(pub_projects, project)
		}
	}
	c.JSON(200, pub_projects)
}

func ClaimTask(queue *mongo.Collection, from_status string, archivist string) *primitive.M {
	filter := bson.M{"status": from_status}
	update := bson.M{
		"$set": bson.M{
			"status":     "PROCESSING",
			"archivist":  archivist,
			"claimed_at": primitive.NewDateTimeFromTime(time.Now().UTC()),
			"updated_at": primitive.NewDateTimeFromTime(time.Now().UTC()),
		}}

	var task primitive.M
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := queue.FindOneAndUpdate(context.TODO(), filter, update, opts).Decode(&task)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		panic(err)
	}
	return &task
}

func v1_claim_task(c *gin.Context) {
	identifier := c.Param("identifier")
	client_version := c.Param("client_version")
	archivist := c.Param("archivist")
	if is_safe_sting(identifier) && is_safe_sting(archivist) {
		// OK
	} else {
		c.JSON(400, gin.H{"error": "Invalid identifier or archivist"})
		return
	}

	project := GetProject(identifier)
	if project == nil {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
		return
	}
	// 暂停后不再接受新的 claim_task 请求。
	if project.Status.Paused {
		c.JSON(400, gin.H{
			"error": "Project paused",
		})
		return
	}
	if client_version != project.Client.Version {
		c.JSON(400, gin.H{
			"error": "Client version not supported",
			"msg":   fmt.Sprintf("Please update to version %s", project.Client.Version),
		})
		return
	}

	db := mongoClient.Database(project.Mongodb.DbName)
	queue := db.Collection(project.Mongodb.QueueCollection)

	task := ClaimTask(queue, "TODO", archivist)
	if task == nil {
		c.JSON(404, gin.H{
			"error": "No task available",
		})
		return
	}
	c.JSON(200, task)
}

func v1_update_task(c *gin.Context) {
	identifier := c.Param("identifier")
	client_version := c.Param("client_version")
	archivist := c.Param("archivist")
	task_id_str := c.Param("task_id")

	status := c.PostForm("status")
	task_id_type := c.PostForm("task_id_type")

	if is_safe_sting(identifier) && is_safe_sting(archivist) {
		// OK
	} else {
		c.JSON(400, gin.H{"error": "Invalid parameter or query string"})
		return
	}

	project := GetProject(identifier)
	if project == nil {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
		return
	}
	if client_version != project.Client.Version {
		c.JSON(400, gin.H{
			"error": "Client version not supported",
			"msg":   fmt.Sprintf("Please update to version %s", project.Client.Version),
		})
		return
	}

	// 为了兼容 lowapk_v2 存档项目。它数据库用的 feed_id 而不是 id
	var doc_id_name string
	if project.Mongodb.CustomDocIDName != "" {
		doc_id_name = project.Mongodb.CustomDocIDName
	} else {
		doc_id_name = DEFAULT_DOC_ID_NAME
	}

	db := mongoClient.Database(project.Mongodb.DbName)
	queue := db.Collection(project.Mongodb.QueueCollection)

	var filter bson.M
	if task_id_type == "int" {
		task_id, _ := strconv.ParseInt(task_id_str, 10, 64)
		filter = bson.M{doc_id_name: task_id}
	} else if task_id_type == "str" {
		filter = bson.M{doc_id_name: task_id_str}
	} else {
		c.JSON(400, gin.H{"error": "Invalid task_id_type"})
		return
	}

	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"archivist":  archivist,
			"updated_at": primitive.NewDateTimeFromTime(time.Now().UTC()),
		}}

	var updated_doc bson.M
	err := queue.FindOneAndUpdate(context.TODO(), filter, update).Decode(&updated_doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(404, gin.H{
				"error": "Task not found",
			})
			return
		}
		panic(err)
	}

	c.JSON(200, gin.H{
		"_id": updated_doc["_id"],
		"msg": "Task updated successfully",
	})
}

func v1_insert_many(c *gin.Context) {
	identifier := c.Param("identifier")
	client_version := c.Param("client_version")
	archivist := c.Param("archivist")

	if is_safe_sting(identifier) && is_safe_sting(archivist) {
		// OK
	} else {
		c.JSON(400, gin.H{"error": "Invalid parameter or query string"})
		return
	}

	project := GetProject(identifier)
	if project == nil {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
		return
	}
	if client_version != project.Client.Version {
		c.JSON(400, gin.H{
			"error": "Client version not supported",
			"msg":   fmt.Sprintf("Please update to version %s", project.Client.Version),
		})
		return
	}

	db := mongoClient.Database(project.Mongodb.DbName)
	item_collection := db.Collection(project.Mongodb.ItemCollection)

	// Parse JSON
	topItems := []Item{}
	if err := c.ShouldBindJSON(&topItems); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	var doc_id_name string
	if project.Mongodb.CustomDocIDName != "" {
		doc_id_name = project.Mongodb.CustomDocIDName
	} else {
		doc_id_name = DEFAULT_DOC_ID_NAME
	}

	documents := []interface{}{}

	for _, item := range topItems {
		document := bson.M{}
		// id
		if item.Item_id_type == "str" {
			document[doc_id_name] = item.Item_id
		} else if item.Item_id_type == "int" {
			item_id_int, err := strconv.ParseInt(item.Item_id, 10, 64)
			if err != nil {
				c.JSON(400, gin.H{"error": "Invalid item_id"})
				return
			}
			document[doc_id_name] = item_id_int
		} else {
			c.JSON(400, gin.H{"error": "Invalid task_id_type"})
			return
		}
		// status
		if item.Item_status_type == "str" {
			document["status"] = item.Item_status
		} else if item.Item_status_type == "int" {
			status, err := strconv.ParseInt(item.Item_status, 10, 64)
			if err != nil {
				c.JSON(400, gin.H{"error": "Invalid item_status"})
				return
			}
			document["status"] = status
		} else if item.Item_status_type == "None" {
			document["status"] = nil
		} else {
			c.JSON(400, gin.H{"error": "Invalid status_type"})
			return
		}
		// payload
		var payload_BSON primitive.M
		err := bson.UnmarshalExtJSON([]byte(item.Payload), true, &payload_BSON)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid JSON payload"})
			panic(err)
		}
		document["payload"] = payload_BSON

		documents = append(documents, document)
	}

	// do insert, sorted=false
	opt := options.InsertMany().SetOrdered(false)
	result, err := item_collection.InsertMany(context.TODO(), documents, opt)
	// if err is BulkWriteException
	if err != nil {
		if !errors.As(err, &mongo.BulkWriteException{}) {
			c.JSON(500, gin.H{"error": "Failed to insert items"})
			panic(err)
		}
		// BulkWriteException is expected
	}

	bulkWriteException, _ := err.(mongo.BulkWriteException)
	c.JSON(200, gin.H{
		"InsertedIDs":       result.InsertedIDs,
		"msg":               "Items bulk insert actions done successfully",
		"WriteErrors":       len(bulkWriteException.WriteErrors),
		"Labels":            len(bulkWriteException.Labels),
		"WriteConcernError": bulkWriteException.WriteConcernError,
	})
}

func v1_insert_item(c *gin.Context) {
	identifier := c.Param("identifier")
	client_version := c.Param("client_version")
	archivist := c.Param("archivist")
	item_id_str := c.Param("item_id")

	var item_id_type, item_status, item_status_type, payload string

	if c.ContentType() == "application/json" {
		item := Item{}
		if err := c.ShouldBindJSON(&item); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if item.Item_id != item_id_str {
			c.JSON(400, gin.H{"error": "item_id in URL does not match item_id in JSON"})
			return
		}
		item_id_type = item.Item_id_type
		item_status = item.Item_status
		item_status_type = item.Item_status_type
		payload = item.Payload
	} else {
		item_id_type = c.PostForm("item_id_type")         // str, int
		item_status = c.PostForm("item_status")           // item status
		item_status_type = c.PostForm("item_status_type") // None, str, int
		payload = c.PostForm("payload")                   // Any JSON string
	}

	if is_safe_sting(identifier) && is_safe_sting(archivist) {
		// OK
	} else {
		c.JSON(400, gin.H{"error": "Invalid parameter or query string"})
		return
	}

	project := GetProject(identifier)
	if project == nil {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
		return
	}
	if client_version != project.Client.Version {
		c.JSON(400, gin.H{
			"error": "Client version not supported",
			"msg":   fmt.Sprintf("Please update to version %s", project.Client.Version),
		})
		return
	}

	db := mongoClient.Database(project.Mongodb.DbName)
	item_collection := db.Collection(project.Mongodb.ItemCollection)

	var doc_id_name string
	if project.Mongodb.CustomDocIDName != "" {
		doc_id_name = project.Mongodb.CustomDocIDName
	} else {
		doc_id_name = DEFAULT_DOC_ID_NAME
	}

	document := bson.M{}
	// id
	if item_id_type == "str" {
		document[doc_id_name] = item_id_str
	} else if item_id_type == "int" {
		item_id_int, err := strconv.ParseInt(item_id_str, 10, 64)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid item_id"})
			return
		}
		document[doc_id_name] = item_id_int
	} else {
		c.JSON(400, gin.H{"error": "Invalid task_id_type"})
		return
	}
	// status
	if item_status_type == "str" {
		document["status"] = item_status
	} else if item_status_type == "int" {
		status, err := strconv.ParseInt(item_status, 10, 64)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid item_status"})
			return
		}
		document["status"] = status
	} else if item_status_type == "None" {
		document["status"] = nil
	} else {
		c.JSON(400, gin.H{"error": "Invalid status_type"})
		return
	}
	// payload
	var payload_BSON primitive.M
	err := bson.UnmarshalExtJSON([]byte(payload), true, &payload_BSON)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid JSON payload"})
		panic(err)
	}
	document["payload"] = payload_BSON

	// do insert
	result, err := item_collection.InsertOne(context.TODO(), document)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(500, gin.H{"error": "Failed to insert item, duplicate key"})
			return
		}
		c.JSON(500, gin.H{"error": "Failed to insert item"})
		panic(err)
	}
	if result.InsertedID == nil {
		c.JSON(500, gin.H{"error": "Failed to insert item"})
		panic("Failed to insert item")
	}

	c.JSON(200, gin.H{
		"_id": result.InsertedID,
		"msg": "Item inserted successfully",
	})
}
