package actions

import (
	"fmt"
	"gosrc/logger"
	"gosrc/parser"
	"sync"
)

// ═══════════════════════════════════════════════════════
//  F1 — Per-Project Deployment Queue
//  Only one deployment runs per project at a time.
//  Latest pending commit wins — intermediate commits are discarded.
// ═══════════════════════════════════════════════════════

type PendingDeploy struct {
	CommitHash string
	CommitMsg  string
	Config     *parser.Config
}

type ProjectQueue struct {
	mu        sync.Mutex
	isRunning bool
	pending   *PendingDeploy // only one slot — latest overwrites
}

// queues is the global registry of per-project deployment queues.
// Key: ServiceDir (unique per project)
var queues sync.Map

func getQueue(serviceDir string) *ProjectQueue {
	val, _ := queues.LoadOrStore(serviceDir, &ProjectQueue{})
	return val.(*ProjectQueue)
}

// EnqueueDeployment tries to start a deployment. If one is already running for this project,
// it stores the latest pending deploy (overwriting any previous pending).
// The deployFn is the actual deployment work (sync + install + run).
// Returns true if the deployment was started immediately, false if it was queued.
func EnqueueDeployment(cfg *parser.Config, hash, msg string, deployFn func(*parser.Config, string, string)) bool {
	q := getQueue(cfg.ServiceDir)
	q.mu.Lock()

	if q.isRunning {
		// A deploy is already running — queue the latest (overwrite any pending)
		q.pending = &PendingDeploy{
			CommitHash: hash,
			CommitMsg:  msg,
			Config:     cfg,
		}
		q.mu.Unlock()
		logger.Info(fmt.Sprintf("📋 Queued deployment for %s (commit %s). Waiting for current deploy to finish.", cfg.ServiceName, truncHash(hash)), cfg)
		return false
	}

	// No deploy running — start immediately
	q.isRunning = true
	q.mu.Unlock()

	go func() {
		runQueuedDeploy(q, cfg, hash, msg, deployFn)
	}()
	return true
}

// runQueuedDeploy runs the deploy and then checks for any pending deploys to process
func runQueuedDeploy(q *ProjectQueue, cfg *parser.Config, hash, msg string, deployFn func(*parser.Config, string, string)) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(fmt.Sprintf("🔥 Deploy panic recovered: %v", r), cfg)
		}
	}()

	// Run the actual deployment
	deployFn(cfg, hash, msg)

	// Check if there's a pending deploy to process next
	for {
		q.mu.Lock()
		if q.pending == nil {
			q.isRunning = false
			q.mu.Unlock()
			return
		}

		// Pop the pending deploy
		next := q.pending
		q.pending = nil
		q.mu.Unlock()

		logger.Info(fmt.Sprintf("⏭️  Processing queued deployment for %s (commit %s)", next.Config.ServiceName, truncHash(next.CommitHash)), next.Config)
		deployFn(next.Config, next.CommitHash, next.CommitMsg)
	}
}

// IsDeploymentRunning returns true if a deployment is currently in progress for this project
func IsDeploymentRunning(serviceDir string) bool {
	q := getQueue(serviceDir)
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.isRunning
}

func truncHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
