// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build !no_mongo

package storage

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Global map to hold clients, keyed by source name
var mongoClients = make(map[string]*mongo.Client)
var mongoIndexOnce sync.Map

// InitMongoClients initializes connections based on config
func InitMongoClients(configs []struct{ Name, URI string }) error {
	for _, cfg := range configs {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.URI))
		cancel()
		if err != nil {
			return fmt.Errorf("failed to connect to mongo %s: %w", cfg.Name, err)
		}
		// Verify connection
		ctxPing, cancelPing := context.WithTimeout(context.Background(), 2*time.Second)
		if err := client.Ping(ctxPing, nil); err != nil {
			cancelPing()
			return fmt.Errorf("failed to ping mongo %s: %w", cfg.Name, err)
		}
		cancelPing()
		mongoClients[cfg.Name] = client
		slog.Info("Connected to Mongo source", "source", cfg.Name)
	}
	return nil
}

// CloseAllMongoClients disconnects all mongo clients gracefully.
func CloseAllMongoClients() {
	for name, client := range mongoClients {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := client.Disconnect(ctx); err != nil {
			slog.Warn("Failed to disconnect mongo client", "source", name, "error", err)
		} else {
			slog.Info("Disconnected mongo client", "source", name)
		}
		cancel()
	}
}

type MongoStorage struct {
	client     *mongo.Client
	dbName     string
	collName   string
	collection *mongo.Collection
}

func NewMongoStorage(config map[string]string) (*MongoStorage, error) {
	sourceName := config["source"]
	dbName := config["database"]
	collName := config["collection"]

	if sourceName == "" || dbName == "" || collName == "" {
		return nil, fmt.Errorf("mongo storage requires 'source', 'database', and 'collection' config")
	}

	client, ok := mongoClients[sourceName]
	if !ok {
		return nil, fmt.Errorf("mongo source '%s' not configured", sourceName)
	}

	st := &MongoStorage{
		client:     client,
		dbName:     dbName,
		collName:   collName,
		collection: client.Database(dbName).Collection(collName),
	}
	if err := st.ensureIndexes(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *MongoStorage) Get(id string) (*model.DownloadObject, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Assuming 'url' is the unique ID for now, as per simple_task logic
	filter := bson.M{"url": id}
	var obj model.DownloadObject
	err := s.collection.FindOne(ctx, filter).Decode(&obj)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &obj, nil
}

func (s *MongoStorage) Update(obj *model.DownloadObject) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"url": obj.URL}
	update := bson.M{"$set": obj}
	opts := options.Update().SetUpsert(true)

	_, err := s.collection.UpdateOne(ctx, filter, update, opts)
	return err
}

func (s *MongoStorage) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.collection.DeleteOne(ctx, bson.M{"url": id})
	return err
}

func (s *MongoStorage) Search(query *core.StorageQuery) ([]*model.DownloadObject, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query = normalizeMongoQuery(query)
	filter := buildMongoFilter(query)
	opts := options.Find()
	if query.Offset > 0 {
		opts.SetSkip(int64(query.Offset))
	}
	if query.Limit > 0 {
		opts.SetLimit(int64(query.Limit))
	}
	if sortDoc := buildMongoSort(query.Sort); len(sortDoc) > 0 {
		opts.SetSort(sortDoc)
	}

	cursor, err := s.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	results := make([]*model.DownloadObject, 0)
	for cursor.Next(ctx) {
		var obj model.DownloadObject
		if err := cursor.Decode(&obj); err != nil {
			return nil, err
		}
		results = append(results, &obj)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *MongoStorage) Count(query *core.StorageQuery) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := s.collection.CountDocuments(ctx, buildMongoFilter(normalizeMongoQuery(query)))
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *MongoStorage) Exists(ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	for _, id := range ids {
		result[id] = false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := s.collection.Find(ctx, bson.M{"url": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"url": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var row struct {
			URL string `bson:"url"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.URL] = true
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *MongoStorage) ensureIndexes() error {
	if s == nil || s.collection == nil {
		return nil
	}
	key := s.dbName + "." + s.collName
	if _, loaded := mongoIndexOnce.LoadOrStore(key, struct{}{}); loaded {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "url", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("url_unique"),
		},
		{
			Keys:    bson.D{{Key: "task_id", Value: 1}, {Key: "status", Value: 1}},
			Options: options.Index().SetName("task_status"),
		},
		{
			Keys:    bson.D{{Key: "task_id", Value: 1}, {Key: "metadata.content_group", Value: 1}},
			Options: options.Index().SetName("task_group"),
		},
		{
			Keys:    bson.D{{Key: "task_id", Value: 1}, {Key: "metadata.date", Value: -1}},
			Options: options.Index().SetName("task_date_desc"),
		},
		{
			Keys:    bson.D{{Key: "metadata.title", Value: 1}},
			Options: options.Index().SetName("title_lookup"),
		},
	}
	if _, err := s.collection.Indexes().CreateMany(ctx, models); err != nil {
		mongoIndexOnce.Delete(key)
		return fmt.Errorf("failed to ensure mongo indexes for %s: %w", key, err)
	}
	return nil
}

func buildMongoFilter(query *core.StorageQuery) bson.M {
	filter := bson.M{}
	if query == nil {
		return filter
	}
	if len(query.Filter.TaskIDs) > 0 {
		filter["task_id"] = bson.M{"$in": query.Filter.TaskIDs}
	}
	if len(query.Filter.URLs) > 0 {
		filter["url"] = bson.M{"$in": query.Filter.URLs}
	}
	if len(query.Filter.Statuses) > 0 {
		filter["status"] = bson.M{"$in": query.Filter.Statuses}
	}
	for key, value := range query.Filter.Metadata {
		filter["metadata."+key] = value
	}
	if query.Filter.Search != "" {
		pattern := regexp.QuoteMeta(query.Filter.Search)
		filter["$or"] = bson.A{
			bson.M{"url": bson.M{"$regex": pattern, "$options": "i"}},
			bson.M{"metadata.title": bson.M{"$regex": pattern, "$options": "i"}},
			bson.M{"extra.tags": bson.M{"$regex": pattern, "$options": "i"}},
		}
	}
	return filter
}

func normalizeMongoQuery(query *core.StorageQuery) *core.StorageQuery {
	if query == nil {
		return &core.StorageQuery{
			Limit: 200,
			Sort:  []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}},
		}
	}
	cloned := *query
	cloned.Filter.TaskIDs = append([]string(nil), query.Filter.TaskIDs...)
	cloned.Filter.URLs = append([]string(nil), query.Filter.URLs...)
	cloned.Filter.Statuses = append([]string(nil), query.Filter.Statuses...)
	if query.Filter.Metadata != nil {
		cloned.Filter.Metadata = make(map[string]string, len(query.Filter.Metadata))
		maps.Copy(cloned.Filter.Metadata, query.Filter.Metadata)
	}
	cloned.Sort = append([]core.StorageSort(nil), query.Sort...)
	if cloned.Limit <= 0 {
		cloned.Limit = 200
	}
	if cloned.Limit > 1000 {
		cloned.Limit = 1000
	}
	if len(cloned.Sort) == 0 {
		cloned.Sort = []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}}
	}
	return &cloned
}

func buildMongoSort(sorts []core.StorageSort) bson.D {
	order := bson.D{}
	for _, sortRule := range sorts {
		field := mongoSortField(sortRule.Field)
		if field == "" {
			continue
		}
		direction := 1
		if sortRule.Desc {
			direction = -1
		}
		order = append(order, bson.E{Key: field, Value: direction})
	}
	return order
}

func mongoSortField(field string) string {
	switch field {
	case "date":
		return "metadata.date"
	case "name":
		return "metadata.title"
	case "duration":
		return "metadata.duration"
	case "status":
		return "status"
	case "url":
		return "url"
	default:
		return ""
	}
}
