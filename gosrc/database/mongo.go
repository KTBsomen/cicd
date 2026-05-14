package database

import (
	"context"
	"fmt"
	"time"

	"gosrc/logger"
	"gosrc/parser"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var mongoClient *mongo.Client

func InitMongo(ctx context.Context) error {
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	mongoClient = client
	return nil
}

// RegisterToMongo syncs the local configuration to your centralized MongoDB
func RegisterToMongo(cfg *parser.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDBURI))
	if err != nil {
		logger.Error("Failed to connect to MongoDB", cfg)
		return err
	}
	defer client.Disconnect(ctx)
	collection := client.Database("cicd").Collection("projects")
	// Filter by Repo + Branch + PublicIP to identify this specific server/project
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
		logger.Error("Failed to update MongoDB record", cfg)
		return err
	}
	logger.Info("☁️  Centralized config synced to MongoDB", cfg)
	return nil
}
func UpdateCommitHash(cfg *parser.Config, commitHash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDBURI))
	if err != nil {
		logger.Error("Failed to connect to MongoDB", cfg)
		return err
	}
	defer client.Disconnect(ctx)
	collection := client.Database("cicd").Collection("projects")
	// Filter by Repo + Branch + PublicIP to identify this specific server/project
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
		logger.Error("Failed to update MongoDB record", cfg)
		return err
	}
	logger.Info("☁️  Centralized config synced to MongoDB", cfg)
	return nil
}

// WatchChanges sits in a background loop and waits for Dashboard updates
func WatchChanges(cfg *parser.Config, callback func(*parser.Config)) {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDBURI))
	if err != nil {
		return
	}
	collection := client.Database("cicd").Collection("projects")

	// We only want to watch changes for THIS server
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"fullDocument.repo_url": cfg.RepoURL}}},
	}
	opts := options.ChangeStream().SetFullDocument(options.UpdateLookup)
	stream, err := collection.Watch(ctx, pipeline, opts)
	if err != nil {
		logger.Error("Failed to start MongoDB Watch Stream", cfg)
		return
	}
	defer stream.Close(ctx)
	logger.Info("📡 Watching MongoDB for remote config changes...", cfg)
	for stream.Next(ctx) {
		var event bson.M
		if err := stream.Decode(&event); err == nil {
			fmt.Println("changes detected raw event:", event)
			// Extract the project name and notify the orchestrator
			fullDoc := event["fullDocument"].(bson.M)
			projectName := fullDoc["service_name"].(string)
			repoURL := fullDoc["repo_url"].(string)
			project, err := GetProjectByRepoURL(repoURL)
			if err != nil {
				return
			}
			logger.Info("🔔 Remote change detected for: "+projectName, project.ToConfig())
			callback(project.ToConfig()) // This will trigger our internal "TriggerUpdate"
		}
	}
}

// GetUniqueServerIPs retrieves all unique public_ips from the centralized registry
func GetUniqueServerIPs(cfg *parser.Config) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDBURI))
	if err != nil {
		logger.Error("Failed to connect to MongoDB", cfg)
		return nil, err
	}
	defer client.Disconnect(ctx)

	collection := client.Database("cicd").Collection("projects")
	values, err := collection.Distinct(context.Background(), "public_ips", bson.M{})
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
