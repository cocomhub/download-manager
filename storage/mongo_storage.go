// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/cocomhub/download-manager/model"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Global map to hold clients, keyed by source name
var mongoClients = make(map[string]*mongo.Client)

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

	return &MongoStorage{
		client:     client,
		dbName:     dbName,
		collName:   collName,
		collection: client.Database(dbName).Collection(collName),
	}, nil
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

func (s *MongoStorage) Search(filter any) ([]*model.DownloadObject, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Handle filter. If nil, empty filter (all)
	bsonFilter := bson.M{}
	if f, ok := filter.(map[string]any); ok {
		maps.Copy(bsonFilter, f)
	} else if f, ok := filter.(bson.M); ok {
		bsonFilter = f
	}

	cursor, err := s.collection.Find(ctx, bsonFilter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*model.DownloadObject
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}
