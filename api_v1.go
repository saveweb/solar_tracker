package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ID = "id"

func v1_project(c echo.Context) error {
	identifier := c.Param("identifier")
	if !is_safe_sting(identifier) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid identifier"})
	}
	project := GetProject(identifier)
	if project == nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
	}
	return c.JSON(http.StatusOK, project)
}

func v1_projects(c echo.Context) error {
	projects := GetProjects()
	pub_projects := []Project{}
	show_private := c.QueryParam("show_private")
	if show_private != "" {
		return c.JSON(http.StatusOK, projects)
	}

	for _, project := range projects {
		if project.Status.Public {
			pub_projects = append(pub_projects, project)
		}
	}
	return c.JSON(http.StatusOK, pub_projects)
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

func v1_claim_task(c echo.Context) error {
	identifier := c.Param("identifier")
	client_version := c.Param("client_version")
	archivist := c.Param("archivist")
	if is_safe_sting(identifier) && is_safe_sting(archivist) {
		// OK
	} else {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid identifier or archivist"})
	}

	project := GetProject(identifier)
	if project == nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
	}
	// 暂停后不再接受新的 claim_task 请求。
	if project.Status.Paused {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "Project paused",
		})
	}
	if client_version != project.Client.Version {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "Client version not supported",
			"msg":   fmt.Sprintf("Please update to version %s", project.Client.Version),
		})
	}

	db := mongoClient.Database(project.Mongodb.DbName)
	queue := db.Collection(project.Mongodb.QueueCollection)

	task := ClaimTask(queue, "TODO", archivist)
	if task == nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": "No task available",
		})
	}

	select {
	case archiveEventChan <- ArchiveEvent{
		ProjectID: project.Meta.Identifier,
		Archivist: archivist,
		Message:   fmt.Sprintf("claimed task:%v", (*task)["id"]),
		Tasks:     1,
		Archived:  false,
	}:
	default:
		fmt.Println("archiveEventChan is full, dropping event")
	}

	return c.JSON(http.StatusOK, task)
}

func v1_update_task(c echo.Context) error {
	identifier := c.Param("identifier")
	client_version := c.Param("client_version")
	archivist := c.Param("archivist")
	task_id_str := c.Param("task_id")

	status := c.FormValue("status")
	task_id_type := c.FormValue("task_id_type")

	if is_safe_sting(identifier) && is_safe_sting(archivist) {
		// OK
	} else {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid parameter or query string"})
	}

	project := GetProject(identifier)
	if project == nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
	}
	if client_version != project.Client.Version {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "Client version not supported",
			"msg":   fmt.Sprintf("Please update to version %s", project.Client.Version),
		})
	}

	db := mongoClient.Database(project.Mongodb.DbName)
	queue := db.Collection(project.Mongodb.QueueCollection)

	var filter bson.M
	switch task_id_type {
	case "int":
		task_id, _ := strconv.ParseInt(task_id_str, 10, 64)
		filter = bson.M{ID: task_id}
	case "str":
		filter = bson.M{ID: task_id_str}
	default:
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid task_id_type"})
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
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "Task not found",
			})
		}
		panic(err)
	}

	select {
	case archiveEventChan <- ArchiveEvent{
		ProjectID: project.Meta.Identifier,
		Archivist: archivist,
		Message:   fmt.Sprintf("set task:%s to status:%s", task_id_str, status),
		Tasks:     1,
		Archived:  false,
	}:
	default:
		fmt.Println("archiveEventChan is full, dropping event")
	}

	return c.JSON(http.StatusOK, echo.Map{
		"_id": updated_doc["_id"],
		"msg": "Task updated successfully",
	})
}

func v1_insert_item(c echo.Context) error {
	identifier := c.Param("identifier")
	client_version := c.Param("client_version")
	archivist := c.Param("archivist")
	item_id_str := c.Param("item_id")

	var item_id_type, item_status, item_status_type, payload string

	if strings.HasPrefix(strings.ToLower(c.Request().Header.Get(echo.HeaderContentType)), echo.MIMEApplicationJSON) {
		item := Item{}
		if err := c.Bind(&item); err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{"error": err.Error()})
		}
		if item.Item_id != item_id_str {
			return c.JSON(http.StatusBadRequest, echo.Map{"error": "item_id in URL does not match item_id in JSON"})
		}
		item_id_type = item.Item_id_type
		item_status = item.Item_status
		item_status_type = item.Item_status_type
		payload = item.Payload
	} else {
		item_id_type = c.FormValue("item_id_type")         // str, int
		item_status = c.FormValue("item_status")           // item status
		item_status_type = c.FormValue("item_status_type") // None, str, int
		payload = c.FormValue("payload")                   // Any JSON string
	}

	if is_safe_sting(identifier) && is_safe_sting(archivist) {
		// OK
	} else {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid parameter or query string"})
	}

	project := GetProject(identifier)
	if project == nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
	}
	if client_version != project.Client.Version {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "Client version not supported",
			"msg":   fmt.Sprintf("Please update to version %s", project.Client.Version),
		})
	}

	db := mongoClient.Database(project.Mongodb.DbName)
	item_collection := db.Collection(project.Mongodb.ItemCollection)

	document := bson.M{}
	// id
	switch item_id_type {
	case "str":
		document[ID] = item_id_str
	case "int":
		item_id_int, err := strconv.ParseInt(item_id_str, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid item_id"})
		}
		document[ID] = item_id_int
	default:
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid task_id_type", "item_id_type": item_id_type})
	}
	// status
	switch item_status_type {
	case "str":
		document["status"] = item_status
	case "int":
		status, err := strconv.ParseInt(item_status, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid item_status"})
		}
		document["status"] = status
	case "None":
		document["status"] = nil
	default:
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid status_type"})
	}
	// payload
	var payload_BSON primitive.M
	err := bson.UnmarshalExtJSON([]byte(payload), true, &payload_BSON)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid JSON payload"})
	}
	document["payload"] = payload_BSON

	// do insert
	result, err := item_collection.InsertOne(context.TODO(), document)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return c.JSON(http.StatusOK, echo.Map{"error": "Failed to insert item, duplicate key"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "Failed to insert item"})
	}
	if result.InsertedID == nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "Failed to insert item"})
	}

	select {
	case archiveEventChan <- ArchiveEvent{
		ProjectID: project.Meta.Identifier,
		Archivist: archivist,
		Archived:  true,
		Message:   fmt.Sprintf("inserted item:%v", document[ID]),
		Tasks:     1,
	}:
	default:
		fmt.Println("archiveEventChan is full, dropping event")
	}

	return c.JSON(http.StatusOK, echo.Map{
		"_id": result.InsertedID,
		"msg": "Item inserted successfully",
	})
}

func v1_insert_many(c echo.Context) error {
	identifier := c.Param("identifier")
	client_version := c.Param("client_version")
	archivist := c.Param("archivist")

	if is_safe_sting(identifier) && is_safe_sting(archivist) {
		// OK
	} else {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid parameter or query string"})
	}

	project := GetProject(identifier)
	if project == nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": fmt.Sprintf("Project %s not found", identifier),
		})
	}
	if client_version != project.Client.Version {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "Client version not supported",
			"msg":   fmt.Sprintf("Please update to version %s", project.Client.Version),
		})
	}

	db := mongoClient.Database(project.Mongodb.DbName)
	item_collection := db.Collection(project.Mongodb.ItemCollection)

	// Parse JSON
	topItems := []Item{}
	if err := c.Bind(&topItems); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": err.Error()})
	}

	documents := []any{}

	for _, item := range topItems {
		document := bson.M{}
		// id
		switch item.Item_id_type {
		case "str":
			document[ID] = item.Item_id
		case "int":
			item_id_int, err := strconv.ParseInt(item.Item_id, 10, 64)
			if err != nil {
				return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid item_id"})
			}
			document[ID] = item_id_int
		default:
			return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid task_id_type"})
		}
		// status
		switch item.Item_status_type {
		case "str":
			document["status"] = item.Item_status
		case "int":
			status, err := strconv.ParseInt(item.Item_status, 10, 64)
			if err != nil {
				return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid item_status"})
			}
			document["status"] = status
		case "None":
			document["status"] = nil
		default:
			return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid status_type"})
		}
		// payload
		var payload_BSON primitive.M
		err := bson.UnmarshalExtJSON([]byte(item.Payload), true, &payload_BSON)
		if err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid JSON payload"})
		}
		document["payload"] = payload_BSON

		documents = append(documents, document)
	}

	// do insert, sorted=false
	opt := options.InsertMany().SetOrdered(false)
	result, err := item_collection.InsertMany(context.TODO(), documents, opt)
	// if err is BulkWriteException
	if err != nil && !errors.As(err, &mongo.BulkWriteException{}) {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "Failed to insert items"})
	}

	// BulkWriteException is expected in case of some duplicate key errors
	bulkWriteException, _ := err.(mongo.BulkWriteException)

	select {
	case archiveEventChan <- ArchiveEvent{
		ProjectID: project.Meta.Identifier,
		Archivist: archivist,
		Archived:  true,
		Message:   fmt.Sprintf("tried %d items, inserted %d items", len(topItems), len(result.InsertedIDs)),
	}:
	default:
		fmt.Println("archiveEventChan is full, dropping event")
	}

	return c.JSON(http.StatusOK, echo.Map{
		"InsertedIDs":       result.InsertedIDs,
		"msg":               "Items bulk insert actions done successfully",
		"WriteErrors":       len(bulkWriteException.WriteErrors),
		"Labels":            len(bulkWriteException.Labels),
		"WriteConcernError": bulkWriteException.WriteConcernError,
	})
}
