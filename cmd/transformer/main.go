package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mitm_transformation/internal/crypto"
	"mitm_transformation/internal/db"
	"mitm_transformation/internal/engine"
	"mitm_transformation/internal/engine/transform"
	"mitm_transformation/internal/engine/validate"
)

var (
	appName        = "Transformation Engine"
	appDescription = "Applies mapping rules and transformations to data"
	version        = "0.15.0"
)

// IPCClient is used to send events to the scheduler
type IPCClient struct {
	SocketPath string
	RunID      int
	Component  string
	Topic      string
	SourceName string
}

func (c *IPCClient) SendEvent(status, message string, progress int) {
	if c == nil || c.SocketPath == "" {
		return
	}
	conn, err := net.Dial("unix", c.SocketPath)
	if err != nil {
		log.Printf("[IPC ERROR] Failed to connect to scheduler socket: %v", err)
		return
	}
	defer conn.Close()

	if c.Topic != "" && c.SourceName != "" {
		message = fmt.Sprintf("%s: %s: %s", c.Topic, c.SourceName, message)
	}

	event := map[string]interface{}{
		"run_id":   c.RunID,
		"type":     "status",
		"status":   status,
		"message":  message,
		"progress": progress,
	}
	data, _ := json.Marshal(event)
	_, _ = conn.Write(append(data, '\n'))
}

func (c *IPCClient) SendAudit(message string) {
	if c == nil || c.SocketPath == "" {
		return
	}
	conn, err := net.Dial("unix", c.SocketPath)
	if err != nil {
		log.Printf("[IPC ERROR] Failed to connect to scheduler socket: %v", err)
		return
	}
	defer conn.Close()

	if c.Topic != "" && c.SourceName != "" {
		message = fmt.Sprintf("%s: %s: %s", c.Topic, c.SourceName, message)
	}

	event := map[string]interface{}{
		"run_id":    c.RunID,
		"type":      "audit",
		"component": c.Component,
		"message":   message,
	}
	data, _ := json.Marshal(event)
	_, _ = conn.Write(append(data, '\n'))
}

type DBConnectionConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type DBConfig struct {
	DB DBConnectionConfig `json:"db"`
}

type JobArgs struct {
	BatchSize       int      `json:"batch_size"`
	Workers         int      `json:"workers"`
	RetryFailed     bool     `json:"retry_failed"`
	Topic           string   `json:"topic"`
	RequiredSources []string `json:"required_sources"`
	SourceName      string   `json:"source_name"`
}

func main() {
	version = strings.Split(version, "-")[0]

	var ipc *IPCClient
	runIDStr := os.Getenv("RUN_ID")
	socketPath := os.Getenv("SCHEDULER_SOCKET_PATH")
	if runIDStr != "" && socketPath != "" {
		runID, err := strconv.Atoi(runIDStr)
		if err == nil {
			ipc = &IPCClient{
				SocketPath: socketPath,
				RunID:      runID,
				Component:  "mitm_transformation",
			}
		}
	}

	ipc.SendEvent("started", fmt.Sprintf("%s (%s) started", appName, version), 0)
	ipc.SendAudit(fmt.Sprintf("%s (%s) started", appName, version))

	var dbCfg DBConfig

	// Read DB configuration
	configSource := "Environment Variables"
	jsonConfig := os.Getenv("MITM_DB_CONFIG_JSON")

	if jsonConfig != "" {
		if err := json.Unmarshal([]byte(jsonConfig), &dbCfg); err != nil {
			log.Fatalf("Failed to parse DB config from MITM_DB_CONFIG_JSON: %v", err)
		}
		configSource = "JSON Config (MITM_DB_CONFIG_JSON)"
	} else {
		dbCfg.DB.Host = os.Getenv("MITM_DB_HOST")
		if portStr := os.Getenv("MITM_DB_PORT"); portStr != "" {
			fmt.Sscanf(portStr, "%d", &dbCfg.DB.Port)
		}
		dbCfg.DB.User = os.Getenv("MITM_DB_USER")
		dbCfg.DB.Password = os.Getenv("MITM_DB_PASSWORD")
		dbCfg.DB.Database = os.Getenv("MITM_DB_NAME")
	}

	if dbCfg.DB.Host == "" {
		log.Fatal("MitM database credentials not found in environment (MITM_DB_HOST or MITM_DB_CONFIG_JSON)")
	}

	if ipc != nil {
		ipc.SendAudit(fmt.Sprintf("Loaded database configuration from %s", configSource))
	}

	jobArgs := JobArgs{
		BatchSize:   500,
		Workers:     5,
		RetryFailed: false,
	}

	if len(os.Args) >= 2 {
		if err := json.Unmarshal([]byte(os.Args[1]), &jobArgs); err != nil {
			log.Printf("Warning: Failed to parse job arguments from os.Args[1]: %v", err)
			ipc.SendAudit(fmt.Sprintf("Warning: Failed to parse job arguments from os.Args[1]: %v", err))
		}
	}

	if jobArgs.BatchSize <= 0 {
		jobArgs.BatchSize = 500
	}
	if jobArgs.Workers <= 0 {
		jobArgs.Workers = 5
	}
	if jobArgs.Topic == "" {
		jobArgs.Topic = "employee.data"
	}
	if len(jobArgs.RequiredSources) == 0 {
		jobArgs.RequiredSources = []string{"ORA_EMPLOYEE"}
	}
	if jobArgs.SourceName == "" {
		jobArgs.SourceName = "TRANSFORMATION"
	}

	if ipc != nil {
		ipc.Topic = jobArgs.Topic
		ipc.SourceName = jobArgs.SourceName
	}

	// 1. Connect to Database
	sslMode := "disable"
	if os.Getenv("MITM_DB_SSLMODE") == "true" {
		sslMode = "require"
	}
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", dbCfg.DB.User, dbCfg.DB.Password, dbCfg.DB.Host, dbCfg.DB.Port, dbCfg.DB.Database, sslMode)
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		ipc.SendAudit(fmt.Sprintf("Unable to connect to database: %v\n", err))
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	// 2. Initialize Repositories & Engine
	mappingRepo := db.NewMappingRepo(pool)
	ingestionRepo := db.NewIngestionRepo(pool)
	targetRepo := db.NewTargetRepo(pool)

	if err := mappingRepo.LoadAndCache(context.Background()); err != nil {
		ipc.SendAudit(fmt.Sprintf("Failed to load mapping rules: %v", err))
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

	log.Printf("Starting Transformation Batch Job (Topic: %s, Required Sources: %v, Workers: %d, Batch Size: %d, Retry Failed: %t)...", jobArgs.Topic, jobArgs.RequiredSources, jobArgs.Workers, jobArgs.BatchSize, jobArgs.RetryFailed)

	var wrappedKey []byte
	query := `
		SELECT sk.wrapped_key
		FROM delivery_targets dt
		JOIN storage_keys sk ON dt.dek_id = sk.id
		WHERE LOWER(dt.topic) = LOWER($1) AND dt.is_active = true
		LIMIT 1
	`
	if err := pool.QueryRow(context.Background(), query, jobArgs.Topic).Scan(&wrappedKey); err != nil {
		log.Printf("Warning: Failed to fetch wrapped key for topic %s: %v", jobArgs.Topic, err)
		ipc.SendAudit(fmt.Sprintf("Warning: Failed to fetch wrapped key for topic %s: %v", jobArgs.Topic, err))
		// Provide a fallback dummy wrapped key to avoid panics if not configured yet
		masterKeyStr := os.Getenv("MASTER_KEY")
		var masterKey []byte
		if masterKeyStr == "" {
			masterKey = make([]byte, 32)
		} else {
			decoded, err := base64.StdEncoding.DecodeString(masterKeyStr)
			if err == nil {
				masterKey = decoded
			} else {
				masterKey = []byte(masterKeyStr)
			}
			if len(masterKey) > 32 {
				masterKey = masterKey[:32]
			} else if len(masterKey) < 32 {
				padded := make([]byte, 32)
				copy(padded, masterKey)
				masterKey = padded
			}
		}
		wrappedKey, _ = crypto.GenerateWrappedDEK(masterKey)
	}

	// 3. Worker Pool Setup
	jobs := make(chan db.AggregatedFragment, jobArgs.BatchSize)
	var wg sync.WaitGroup

	// Define logAudit
	logAudit := func(msg string) {
		log.Printf("AUDIT: %s", msg)
		if ipc != nil {
			ipc.SendAudit(msg)
		}
	}

	// Start workers
	for i := 0; i < jobArgs.Workers; i++ {
		wg.Add(1)
		go worker(ctx, &wg, jobs, pipeline, targetRepo, ruleSet, wrappedKey, logAudit)
	}

	totalProcessed := 0

	// 4. Dispatcher Loop
dispatcherLoop:
	for {
		if ctx.Err() != nil {
			break dispatcherLoop
		}

		fragments, err := ingestionRepo.ClaimAggregatedFragments(ctx, jobArgs.Topic, jobArgs.RequiredSources, jobArgs.BatchSize)
		if err != nil {
			log.Printf("Error claiming aggregated fragments: %v", err)
			ipc.SendAudit(fmt.Sprintf("Error claiming aggregated fragments: %v", err))
			time.Sleep(2 * time.Second)
			continue
		}

		if len(fragments) == 0 {
			log.Printf("No more fragments to process for Topic '%s' (Required Sources: %v). Batch job complete.", jobArgs.Topic, jobArgs.RequiredSources)
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
	ipc.SendAudit(fmt.Sprintf("Transformation Batch Job finished successfully. Processed %d records.", totalProcessed))
	ipc.SendEvent("finished", fmt.Sprintf("Transformation Batch Job finished successfully. Processed %d records.", totalProcessed), 100)
}

func worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan db.AggregatedFragment, pipeline *engine.PipelineEngine, targetRepo *db.TargetRepo, ruleSet *db.RuleSet, wrappedKey []byte, logAudit func(string)) {
	defer wg.Done()

	// Fetch Master Key from Env (fallback to 32 bytes of zeros if not set - for dev)
	masterKeyStr := os.Getenv("MASTER_KEY")
	var masterKey []byte
	if masterKeyStr == "" {
		masterKey = make([]byte, 32)
	} else {
		decoded, err := base64.StdEncoding.DecodeString(masterKeyStr)
		if err == nil {
			masterKey = decoded
		} else {
			masterKey = []byte(masterKeyStr)
		}
		if len(masterKey) > 32 {
			masterKey = masterKey[:32]
		} else if len(masterKey) < 32 {
			padded := make([]byte, 32)
			copy(padded, masterKey)
			masterKey = padded
		}
	}

	for aggFragment := range jobs {
		var payloads []map[string]interface{}
		var decryptionErrs []db.TransformationError

		for _, fragment := range aggFragment.Fragments {
			// 1. Decrypt payload via Envelope Encryption
			decryptedPayload, err := crypto.EnvelopeDecrypt(masterKey, fragment.WrappedKey, fragment.Nonce, fragment.Payload)
			if err != nil {
				decryptionErrs = append(decryptionErrs, db.TransformationError{
					FailedField:  "payload",
					RuleName:     "decryption",
					ErrorMessage: fmt.Sprintf("Failed to decrypt envelope: %v", err),
				})
				continue
			}

			var payload map[string]interface{}
			// 2. Parse decrypted JSON
			decoder := json.NewDecoder(bytes.NewReader(decryptedPayload))
			decoder.UseNumber()
			if err := decoder.Decode(&payload); err != nil {
				decryptionErrs = append(decryptionErrs, db.TransformationError{
					FailedField:  "payload",
					RuleName:     "json_parse",
					ErrorMessage: err.Error(),
				})
				continue
			}
			payloads = append(payloads, payload)
		}

		if len(decryptionErrs) > 0 {
			_ = targetRepo.WriteTargetAndComplete(context.Background(), aggFragment.CorrelationID, aggFragment.Fragments[0].ID, aggFragment.Topic, nil, decryptionErrs, logAudit)
			continue
		}

		// 3. Merge payloads into a Golden Record
		goldenRecord := engine.MergePayloads(payloads...)

		sourceID := ""
		for _, s := range ruleSet.Sources {
			if s.Topic == aggFragment.Topic {
				sourceID = s.ID
				break
			}
		}

		if sourceID == "" {
			dbErrs := []db.TransformationError{{FailedField: "topic", RuleName: "source_lookup", ErrorMessage: "No mapping source found for topic"}}
			_ = targetRepo.WriteTargetAndComplete(context.Background(), aggFragment.CorrelationID, aggFragment.Fragments[0].ID, aggFragment.Topic, nil, dbErrs, logAudit)
			continue
		}

		targetData, pipelineErrs := pipeline.ProcessPayload(goldenRecord, sourceID, ruleSet, masterKey, wrappedKey)

		var dbErrs []db.TransformationError
		for _, pe := range pipelineErrs {
			dbErrs = append(dbErrs, db.TransformationError{
				FailedField:  pe.FailedField,
				RuleName:     pe.RuleName,
				ErrorMessage: pe.ErrorMessage,
			})
		}

		if err := targetRepo.WriteTargetAndComplete(context.Background(), aggFragment.CorrelationID, aggFragment.Fragments[0].ID, aggFragment.Topic, targetData, dbErrs, logAudit); err != nil {
			msg := fmt.Sprintf("Failed to write target for correlation %s: %v", aggFragment.CorrelationID, err)
			log.Println(msg)
			if logAudit != nil {
				logAudit(msg)
			}
		}
	}
}
