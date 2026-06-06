package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mitm_transformation/internal/db"
	"mitm_transformation/internal/engine"
	"mitm_transformation/internal/engine/transform"
	"mitm_transformation/internal/engine/validate"
)

type DBConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type JobArgs struct {
	BatchSize   int  `json:"batch_size"`
	Workers     int  `json:"workers"`
	RetryFailed bool `json:"retry_failed"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <db_config_json> [job_args_json]", os.Args[0])
	}

	var dbCfg DBConfig
	if err := json.Unmarshal([]byte(os.Args[1]), &dbCfg); err != nil {
		log.Fatalf("Failed to parse DB config from os.Args[1]: %v", err)
	}

	jobArgs := JobArgs{
		BatchSize:   500,
		Workers:     5,
		RetryFailed: false,
	}

	if len(os.Args) >= 3 {
		if err := json.Unmarshal([]byte(os.Args[2]), &jobArgs); err != nil {
			log.Printf("Warning: Failed to parse job arguments from os.Args[2]: %v", err)
		}
	}

	if jobArgs.BatchSize <= 0 {
		jobArgs.BatchSize = 500
	}
	if jobArgs.Workers <= 0 {
		jobArgs.Workers = 5
	}

	// 1. Connect to Database
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s", dbCfg.User, dbCfg.Password, dbCfg.Host, dbCfg.Port, dbCfg.Database)
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	// 2. Initialize Repositories & Engine
	mappingRepo := db.NewMappingRepo(pool)
	ingestionRepo := db.NewIngestionRepo(pool)
	targetRepo := db.NewTargetRepo(pool)

	if err := mappingRepo.LoadAndCache(context.Background()); err != nil {
		log.Fatalf("Failed to load mapping rules: %v", err)
	}
	ruleSet := mappingRepo.GetCachedRules()
	log.Printf("Loaded %d sources, %d targets, %d rules", len(ruleSet.Sources), len(ruleSet.TargetFields), len(ruleSet.Rules))

	registry := engine.NewEngineRegistry()
	transform.RegisterAll(registry)
	validate.RegisterAll(registry)
	pipeline := engine.NewPipelineEngine(registry)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Println("Shutting down gracefully...")
		cancel()
	}()

	log.Printf("Starting Transformation Batch Job (Workers: %d, Batch Size: %d, Retry Failed: %t)...", jobArgs.Workers, jobArgs.BatchSize, jobArgs.RetryFailed)

	// 3. Worker Pool Setup
	jobs := make(chan db.RawFragment, jobArgs.BatchSize)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < jobArgs.Workers; i++ {
		wg.Add(1)
		go worker(ctx, &wg, jobs, pipeline, targetRepo, ruleSet)
	}

	totalProcessed := 0

	// 4. Dispatcher Loop
dispatcherLoop:
	for {
		if ctx.Err() != nil {
			break dispatcherLoop
		}

		fragments, err := ingestionRepo.ClaimPendingFragments(ctx, jobArgs.BatchSize, jobArgs.RetryFailed)
		if err != nil {
			log.Printf("Error claiming fragments: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if len(fragments) == 0 {
			log.Println("No more fragments to process. Batch job complete.")
			break dispatcherLoop
		}

		log.Printf("Claimed batch of %d fragments", len(fragments))
		for _, f := range fragments {
			select {
			case jobs <- f:
			case <-ctx.Done():
				break dispatcherLoop
			}
		}
		totalProcessed += len(fragments)
	}

	close(jobs)
	wg.Wait()
	log.Printf("Transformation Batch Job finished successfully. Processed %d records.", totalProcessed)
}

func worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan db.RawFragment, pipeline *engine.PipelineEngine, targetRepo *db.TargetRepo, ruleSet *db.RuleSet) {
	defer wg.Done()

	// Simplified: Mock target key for AES-GCM target encryption
	targetKey := []byte("0123456789abcdef0123456789abcdef")

	for fragment := range jobs {
		var payload map[string]interface{}
		
		if err := json.Unmarshal(fragment.Payload, &payload); err != nil {
			dbErrs := []db.TransformationError{{
				FailedField:  "payload",
				RuleName:     "json_parse",
				ErrorMessage: err.Error(),
			}}
			_ = targetRepo.WriteTargetAndComplete(context.Background(), fragment.ID, fragment.Topic, nil, dbErrs)
			continue
		}

		sourceID := ""
		for _, s := range ruleSet.Sources {
			if s.Name == fragment.Topic {
				sourceID = s.ID
				break
			}
		}

		if sourceID == "" {
			dbErrs := []db.TransformationError{{FailedField: "topic", RuleName: "source_lookup", ErrorMessage: "No mapping source found for topic"}}
			_ = targetRepo.WriteTargetAndComplete(context.Background(), fragment.ID, fragment.Topic, nil, dbErrs)
			continue
		}

		targetData, pipelineErrs := pipeline.ProcessPayload(payload, sourceID, ruleSet, targetKey)

		var dbErrs []db.TransformationError
		for _, pe := range pipelineErrs {
			dbErrs = append(dbErrs, db.TransformationError{
				FailedField:  pe.FailedField,
				RuleName:     pe.RuleName,
				ErrorMessage: pe.ErrorMessage,
			})
		}

		if err := targetRepo.WriteTargetAndComplete(context.Background(), fragment.ID, fragment.Topic, targetData, dbErrs); err != nil {
			log.Printf("Failed to write target for fragment %s: %v", fragment.ID, err)
		}
	}
}
