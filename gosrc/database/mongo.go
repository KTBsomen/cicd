package database

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gosrc/logger"
	"gosrc/parser"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ═══════════════════════════════════════════════════════
//  MongoDB Connection Pooling (IF-1)
//  All functions share a single *mongo.Client via sync.Once
// ═══════════════════════════════════════════════════════

var (
	sharedMongoClient *mongo.Client
	mongoMu           sync.Mutex
	lastMongoURI      string
)

// getMongoClient returns a shared MongoDB client. Thread-safe.
// If the URI changes (e.g., user updates settings), reconnects.
func getMongoClient(uri string) (*mongo.Client, error) {
	mongoMu.Lock()
	defer mongoMu.Unlock()

	// If already connected to the SAME URI, return it
	if sharedMongoClient != nil && lastMongoURI == uri {
		return sharedMongoClient, nil
	}

	// If URI changed or first connect, disconnect old client and reconnect
	if sharedMongoClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = sharedMongoClient.Disconnect(ctx)
		sharedMongoClient = nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Production Hardened Options (IF-1)
	clientOpts := options.Client().ApplyURI(uri).
		SetMaxPoolSize(50).
		SetMinPoolSize(2).
		SetMaxConnIdleTime(10 * time.Minute).
		SetConnectTimeout(20 * time.Second).
		SetServerSelectionTimeout(20 * time.Second).
		SetSocketTimeout(60 * time.Second).
		SetHeartbeatInterval(10 * time.Second).
		SetAppName("cicd-fleet-node")

	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate MongoDB connection: %w", err)
	}

	// Verify connection immediately
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("MongoDB connection failed ping: %w", err)
	}

	sharedMongoClient = client
	lastMongoURI = uri
	return sharedMongoClient, nil
}

// RegisterToMongo syncs the local configuration to your centralized MongoDB
func RegisterToMongo(cfg *parser.Config) error {
	if cfg.MongoDBURI == "" {
		return nil // MongoDB not configured, skip silently
	}

	client, err := getMongoClient(cfg.MongoDBURI)
	if err != nil {
		logger.Error("Failed to connect to MongoDB: "+err.Error(), cfg)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	collection := client.Database("cicd").Collection("projects")
	filter := bson.M{
		"repo_url": cfg.RepoURL,
		"branch":   cfg.Branch,
	}
	update := bson.M{
		"$set": bson.M{
			"service_name": cfg.ServiceName,
			"admin_email":  cfg.AdminEmail,
			"service_dir":  cfg.ServiceDir,
			"setup_type":   cfg.Setup,
			"updated_at":   time.Now(),
			"status":       "online",
		},
		"$addToSet": bson.M{
			"public_ips": cfg.PublicIP,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		logger.Error("Failed to update MongoDB record: "+err.Error(), cfg)
		return err
	}
	logger.Info("☁️  Centralized config synced to MongoDB", cfg)
	return nil
}

func UpdateCommitHash(cfg *parser.Config, commitHash string) error {
	if cfg.MongoDBURI == "" {
		return nil
	}

	client, err := getMongoClient(cfg.MongoDBURI)
	if err != nil {
		logger.Error("Failed to connect to MongoDB: "+err.Error(), cfg)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	collection := client.Database("cicd").Collection("projects")
	filter := bson.M{
		"repo_url": cfg.RepoURL,
		"branch":   cfg.Branch,
	}
	update := bson.M{
		"$set": bson.M{
			"commit_hash": commitHash,
			"updated_at":  time.Now(),
		},
		"$addToSet": bson.M{
			"commit_hashes": commitHash,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err = collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		logger.Error("Failed to update MongoDB record: "+err.Error(), cfg)
		return err
	}
	logger.Info("☁️  Commit hash synced to MongoDB", cfg)
	return nil
}

// WatchChanges sits in a background loop and waits for Dashboard updates.
func WatchChanges(cfg *parser.Config, callback func(*parser.Config)) {
	if cfg.MongoDBURI == "" {
		return
	}

	backoff := 2 * time.Second
	for {
		err := startWatchLoop(cfg, callback)
		if err != nil {
			logger.Error(fmt.Sprintf("📡 MongoDB Watch error: %v. Retrying in %v...", err, backoff), cfg)
			time.Sleep(backoff)
			// Simple exponential backoff
			backoff *= 2
			if backoff > 60*time.Second {
				backoff = 60 * time.Second
			}
			continue
		}
		// If it returns nil, it means it finished normally (unlikely for a watch)
		backoff = 2 * time.Second
	}
}

func startWatchLoop(cfg *parser.Config, callback func(*parser.Config)) error {
	client, err := getMongoClient(cfg.MongoDBURI)
	if err != nil {
		return err
	}

	ctx := context.Background()
	collection := client.Database("cicd").Collection("projects")

	// We only want to watch changes for THIS server
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"fullDocument.repo_url": cfg.RepoURL}}},
	}
	opts := options.ChangeStream().SetFullDocument(options.UpdateLookup)
	stream, err := collection.Watch(ctx, pipeline, opts)
	if err != nil {
		return fmt.Errorf("failed to start change stream: %w", err)
	}
	defer stream.Close(ctx)

	logger.Info("📡 MongoDB Change Stream active", cfg)
	for stream.Next(ctx) {
		var event bson.M
		if err := stream.Decode(&event); err == nil {
			fullDoc, ok := event["fullDocument"].(bson.M)
			if !ok {
				continue
			}
			projectName, ok := fullDoc["service_name"].(string)
			if !ok {
				continue
			}
			repoURL, ok := fullDoc["repo_url"].(string)
			if !ok {
				continue
			}
			project, err := GetProjectByRepoURL(repoURL)
			if err != nil {
				logger.Error("Project not found for repo: "+repoURL, cfg)
				continue
			}
			appCfg, err := project.ToConfig()
			if err != nil {
				logger.Error("Failed to convert project to config: "+err.Error(), cfg)
				continue
			}
			logger.Info("🔔 Remote change detected for: "+projectName, appCfg)
			callback(appCfg)
		}
	}
	return stream.Err()
}

// GetUniqueServerIPs retrieves all unique public_ips from the centralized registry
func GetUniqueServerIPs(cfg *parser.Config) ([]string, error) {
	if cfg.MongoDBURI == "" {
		return nil, nil
	}

	client, err := getMongoClient(cfg.MongoDBURI)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := client.Database("cicd").Collection("projects")
	values, err := collection.Distinct(ctx, "public_ips", bson.M{})
	if err != nil {
		return nil, err
	}

	var ips []string
	for _, v := range values {
		if ip, ok := v.(string); ok {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

func DeleteProjectFromMongo(cfg *parser.Config) ([]string, error) {
	if cfg.MongoDBURI == "" {
		return nil, nil
	}

	client, err := getMongoClient(cfg.MongoDBURI)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := client.Database("cicd").Collection("projects")
	_, err = collection.DeleteOne(ctx, bson.M{
		"repo_url":    cfg.RepoURL,
		"branch":      cfg.Branch,
		"serviceName": cfg.ServiceName,
	})
	if err != nil {
		logger.Error("Failed to delete project from MongoDB: "+err.Error(), cfg)
		return nil, err
	}
	logger.Info("🗑️  Project deleted from MongoDB", cfg)
	return nil, nil
}
