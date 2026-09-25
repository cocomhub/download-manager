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
	"github.com/cocomhub/download-manager/pkg/logutil"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	fieldMetadataTitle = "metadata.title"
	opRegex            = "$regex"
	opOptions          = "$options"
)

// Global map to hold clients, keyed by source name
var mongoClients = make(map[string]*mongo.Client)
var mongoIndexOnce sync.Map

// InitMongoClients initializes connections based on config
func InitMongoClients(configs []struct{ Name, URI string }) error {
	for _, cfg := range configs {
		client, err := mongo.Connect(options.Client().ApplyURI(cfg.URI).SetConnectTimeout(10 * time.Second))
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
			slog.Warn("Failed to disconnect mongo client", "source", name, logutil.LogKeyError, err)
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
	// 用 Snapshot 深拷贝编码：BSON 反射在无 obj.mu 保护下迭代 Metadata/Extra map，
	// 与 soWorker/metadata flusher 等并发写方共享同一对象时可能触发
	// "concurrent map iteration and map write"。Snapshot 在 RLock 下深拷贝，
	// 编码线程安全（见 model/object.go Snapshot）。
	update := bson.M{"$set": obj.Snapshot()}
	opts := options.UpdateOne().SetUpsert(true)

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
	// Light 投影：排除大数组字段（extra.files/images/links），减小传输/解码开销。
	if query.Light {
		opts.SetProjection(bson.M{
			"extra.files":  0,
			"extra.images": 0,
			"extra.links":  0,
		})
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
			Keys:    bson.D{{Key: "id", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true).SetName("id_unique"),
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
			Keys:    bson.D{{Key: fieldMetadataTitle, Value: 1}},
			Options: options.Index().SetName("title_lookup"),
		},
		{
			Keys:    bson.D{{Key: "metadata.collection_id", Value: 1}, {Key: "metadata.collection_title", Value: 1}},
			Options: options.Index().SetName("collection_order"),
		},
		{
			// 版本升级扫描：按任务 + version 过滤旧数据（runVersionUpgrade 的 VersionLT 下推）。
			Keys:    bson.D{{Key: "task_id", Value: 1}, {Key: "version", Value: 1}},
			Options: options.Index().SetName("task_version"),
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
	if len(query.Filter.IDs) > 0 {
		filter["id"] = bson.M{"$in": query.Filter.IDs}
	}
	if len(query.Filter.Statuses) > 0 {
		filter["status"] = bson.M{"$in": query.Filter.Statuses}
	}
	for key, value := range query.Filter.Metadata {
		filter["metadata."+key] = value
	}
	if query.Filter.VersionLT > 0 {
		filter["version"] = bson.M{"$lt": query.Filter.VersionLT}
	}

	var andConditions bson.A

	// MissingID 过滤
	if query.Filter.MissingID != nil {
		if *query.Filter.MissingID {
			// MissingID=true: id 不存在或 id=0
			andConditions = append(andConditions, bson.M{
				"$or": bson.A{
					bson.M{"id": bson.M{"$exists": false}},
					bson.M{"id": 0},
				},
			})
		} else {
			// MissingID=false: id 存在且 id != 0
			andConditions = append(andConditions, bson.M{
				"id": bson.M{"$exists": true, "$ne": 0},
			})
		}
	}

	// Search 过滤
	if query.Filter.Search != "" {
		pattern := regexp.QuoteMeta(query.Filter.Search)
		andConditions = append(andConditions, bson.M{
			"$or": bson.A{
				bson.M{"url": bson.M{opRegex: pattern, opOptions: "i"}},
				bson.M{fieldMetadataTitle: bson.M{opRegex: pattern, opOptions: "i"}},
				bson.M{"extra.tags": bson.M{opRegex: pattern, opOptions: "i"}},
			},
		})
	}

	// Tags 过滤
	if len(query.Filter.Tags) > 0 {
		if query.Filter.TagMode == "all" {
			// 所有标签都要匹配（AND）
			for _, tag := range query.Filter.Tags {
				andConditions = append(andConditions, bson.M{
					"extra.tags": bson.M{opRegex: regexp.QuoteMeta(tag), opOptions: "i"},
				})
			}
		} else {
			// 任一标签匹配（OR）
			tagConditions := bson.A{}
			for _, tag := range query.Filter.Tags {
				tagConditions = append(tagConditions, bson.M{
					"extra.tags": bson.M{opRegex: regexp.QuoteMeta(tag), opOptions: "i"},
				})
			}
			if len(tagConditions) > 0 {
				andConditions = append(andConditions, bson.M{"$or": tagConditions})
			}
		}
	}

	// ExcludeIDs 过滤
	if len(query.Filter.ExcludeIDs) > 0 {
		filter["id"] = bson.M{"$nin": query.Filter.ExcludeIDs}
	}

	if len(andConditions) > 0 {
		filter["$and"] = andConditions
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
	cloned.Filter.Tags = append([]string(nil), query.Filter.Tags...)
	cloned.Filter.ExcludeIDs = append([]int64(nil), query.Filter.ExcludeIDs...)
	cloned.Filter.IDs = append([]int64(nil), query.Filter.IDs...)
	if query.Filter.Metadata != nil {
		cloned.Filter.Metadata = make(map[string]string, len(query.Filter.Metadata))
		maps.Copy(cloned.Filter.Metadata, query.Filter.Metadata)
	}
	cloned.Sort = append([]core.StorageSort(nil), query.Sort...)
	if cloned.Limit <= 0 {
		if cloned.Limit != core.NoLimit { // -1 = unlimited, skip clamp
			cloned.Limit = 200
		} else {
			cloned.Limit = 0
		}
	}
	if cloned.Limit > 1000 && cloned.Limit != core.NoLimit {
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
		return fieldMetadataTitle
	case "duration":
		return "metadata.duration"
	case "status":
		return "status"
	case "url":
		return "url"
	case "random":
		return "" // 内存随机
	case "tag_match_desc":
		return "" // 内存排序
	default:
		return ""
	}
}

// ContentGroupRepresentatives 按 metadata.content_group 分组，每组返回 metadata.date 最大的
// 代表对象，并支持分页。用 mongo 聚合一次完成分组/排序/分页，避免把任务全部对象取回
// 内存再分组（跨任务 AggregateByContent 的 content 模式快路径）。
//
// 代表策略固定为 max-date（框架默认语义）。任务有自定义代表语义（ContentGroupProvider
// VariantScore 变体优先级）时，manager 检测到并走内存路径，不使用此快路径。
//
// search/status 与 buildMongoFilter 同语义；limit <= 0 表示不分页；total 为非空组的去重组数。
func (s *MongoStorage) ContentGroupRepresentatives(taskID string, search, status string, page, limit int64) ([]*model.DownloadObject, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 排除缺失 / null / 空串的 content_group。
	// BSON 排序下 null/数字 都排在字符串之前，$gt "" 一个谓词即可把非字符串与空串一并排除，
	// 且是有界的索引范围查询（比 $nin [null,""] 更适合优化器）。
	match := bson.M{
		"task_id":                taskID,
		"metadata.content_group": bson.M{"$gt": ""},
	}
	if status != "" && status != "all" {
		match["status"] = status
	}
	if search != "" {
		pattern := regexp.QuoteMeta(search)
		match["$and"] = bson.A{bson.M{
			"$or": bson.A{
				bson.M{"url": bson.M{opRegex: pattern, opOptions: "i"}},
				bson.M{fieldMetadataTitle: bson.M{opRegex: pattern, opOptions: "i"}},
				bson.M{"extra.tags": bson.M{opRegex: pattern, opOptions: "i"}},
			},
		}}
	}

	// 去重组数（total）。
	var total int64
	{
		countPipe := mongo.Pipeline{
			bson.D{{Key: "$match", Value: match}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$metadata.content_group"}}}},
			bson.D{{Key: "$count", Value: "total"}},
		}
		cursor, err := s.collection.Aggregate(ctx, countPipe)
		if err != nil {
			return nil, 0, err
		}
		defer cursor.Close(ctx)
		if cursor.Next(ctx) {
			var row struct {
				Total int64 `bson:"total"`
			}
			if err := cursor.Decode(&row); err != nil {
				return nil, 0, err
			}
			total = row.Total
		}
		if err := cursor.Err(); err != nil {
			return nil, 0, err
		}
	}

	// 每组代表 = 组内 metadata.date 最大者（$sort date desc 后取 $first），size 为组内对象数。
	pipe := mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "metadata.date", Value: -1}, {Key: "url", Value: 1}}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$metadata.content_group"},
			{Key: "doc", Value: bson.D{{Key: "$first", Value: "$$ROOT"}}},
			{Key: "size", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "doc.metadata.date", Value: -1}, {Key: "_id", Value: -1}}}},
	}
	if page < 1 {
		page = 1
	}
	if limit > 0 {
		if skip := (page - 1) * limit; skip > 0 {
			pipe = append(pipe, bson.D{{Key: "$skip", Value: skip}})
		}
		pipe = append(pipe, bson.D{{Key: "$limit", Value: limit}})
	}
	// 投影剔除大数组字段与内部 _id，减小传输/解码开销。
	pipe = append(pipe, bson.D{{Key: "$project", Value: bson.D{
		{Key: "doc._id", Value: 0},
		{Key: "doc.extra.files", Value: 0},
		{Key: "doc.extra.images", Value: 0},
		{Key: "doc.extra.links", Value: 0},
	}}})

	cursor, err := s.collection.Aggregate(ctx, pipe)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	results := make([]*model.DownloadObject, 0)
	for cursor.Next(ctx) {
		var row struct {
			Doc  model.DownloadObject `bson:"doc"`
			Size int                  `bson:"size"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, 0, err
		}
		row.Doc.SetGroupSize(row.Size)
		results = append(results, &row.Doc)
	}
	if err := cursor.Err(); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}
