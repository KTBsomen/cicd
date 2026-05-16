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
	mongoOnce         sync.Once
	mongoInitErr      error
	mongoMu           sync.Mutex
	lastMongoURI      string
)

// getMongoClient returns a shared MongoDB client. Thread-safe via sync.Once.
// If the URI changes (e.g., user updates settings), reconnects.
func getMongoClient(uri string) (*mongo.Client, error) {
	mongoMu.Lock()
	defer mongoMu.Unlock()

	// If URI changed, disconnect old client and reconnect
	if sharedMongoClient != nil && lastMongoURI != uri {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		sharedMongoClient.Disconnect(ctx)
		sharedMongoClient = nil
		mongoOnce = sync.Once{} // Reset once so it re-runs
	}

	mongoOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		clientOpts := options.Client().ApplyURI(uri).
			SetMaxPoolSize(10).
			SetMinPoolSize(1).
			SetMaxConnIdleTime(5 * time.Minute)

		client, err := mongo.Connect(ctx, clientOpts)
		if err != nil {
			mongoInitErr = fmt.Errorf("failed to connect to MongoDB: %w", err)
			return
		}

		// Verify connection
		if err := client.Ping(ctx, nil); err != nil {
			mongoInitErr = fmt.Errorf("MongoDB ping failed: %w", err)
			client.Disconnect(ctx)
			return
		}

		sharedMongoClient = client
		lastMongoURI = uri
		mongoInitErr = nil
	})

	return sharedMongoClient, mongoInitErr
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
// IF-5: Uses safe type assertions with comma-ok idiom.
func WatchChanges(cfg *parser.Config, callback func(*parser.Config)) {
	if cfg.MongoDBURI == "" {
		return
	}

	client, err := getMongoClient(cfg.MongoDBURI)
	if err != nil {
		logger.Error("Failed to connect to MongoDB for watch: "+err.Error(), cfg)
		return
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
		logger.Error("Failed to start MongoDB Watch Stream: "+err.Error(), cfg)
		return
	}
	defer stream.Close(ctx)
	logger.Info("📡 Watching MongoDB for remote config changes...", cfg)
	for stream.Next(ctx) {
		var event bson.M
		if err := stream.Decode(&event); err == nil {
			// IF-5: Safe type assertions — skip if types don't match
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
