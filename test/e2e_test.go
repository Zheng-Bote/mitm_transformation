package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

func loadConfig(t *testing.T) Config {
	configPath := "../../../data/config.json"
	b, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	return cfg
}

func setupDatabase(t *testing.T, pool *pgxpool.Pool) {
	_, err := pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS raw_ingestion (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			topic TEXT NOT NULL,
			payload BYTEA NOT NULL,
			nonce TEXT,
			dek_id TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TIMESTAMP NOT NULL DEFAULT now()
		);
	`)
	if err != nil {
		t.Fatalf("failed to create raw_ingestion: %v", err)
	}

	migrationsDir := "../../migrations"
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("failed to read migrations dir: %v", err)
	}
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".sql" {
			content, err := os.ReadFile(filepath.Join(migrationsDir, file.Name()))
			if err != nil {
				t.Fatalf("failed to read migration %s: %v", file.Name(), err)
			}
			_, err = pool.Exec(context.Background(), string(content))
			if err != nil {
				t.Fatalf("failed to execute migration %s: %v", file.Name(), err)
			}
		}
	}

	_, err = pool.Exec(context.Background(), `
        CREATE TABLE IF NOT EXISTS "test_topic" (
            id SERIAL PRIMARY KEY,
            "TargetEmail" JSONB,
            "TargetName" TEXT
        );
    `)
	if err != nil {
		t.Fatalf("failed to create target table: %v", err)
	}
}

func TestE2EBatchJob(t *testing.T) {
	cfg := loadConfig(t)
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	setupDatabase(t, pool)

	// Clean up
	pool.Exec(context.Background(), `TRUNCATE raw_ingestion, mapping_rule, mapping_target_field, mapping_source, transformation_errors, "test_topic" CASCADE`)

	// Insert mock rules
	srcID := "00000000-0000-0000-0000-000000000001"
	_, err = pool.Exec(context.Background(), `INSERT INTO mapping_source (id, name, type) VALUES ($1, 'test_topic', 'json')`, srcID)
	if err != nil {
		t.Fatalf("insert source: %v", err)
	}

	tf1ID := "00000000-0000-0000-0000-000000000002"
	_, err = pool.Exec(context.Background(), `INSERT INTO mapping_target_field (id, topic, field_name, data_type, is_required, encrypted) VALUES ($1, 'test_topic', 'TargetEmail', 'jsonb', true, true)`, tf1ID)
	if err != nil {
		t.Fatalf("insert target field 1: %v", err)
	}

	tf2ID := "00000000-0000-0000-0000-000000000003"
	_, err = pool.Exec(context.Background(), `INSERT INTO mapping_target_field (id, topic, field_name, data_type, is_required, encrypted) VALUES ($1, 'test_topic', 'TargetName', 'text', true, false)`, tf2ID)
	if err != nil {
		t.Fatalf("insert target field 2: %v", err)
	}

	rule1ID := "00000000-0000-0000-0000-000000000004"
	_, err = pool.Exec(context.Background(), `INSERT INTO mapping_rule (id, source_id, target_field_id, source_field, transformation_chain, validation_chain) VALUES ($1, $2, $3, 'raw_email', '[{"name": "trim_whitespace", "parameters": {}}]', '[{"name": "email", "parameters": {}}]')`, rule1ID, srcID, tf1ID)
	if err != nil {
		t.Fatalf("insert rule 1: %v", err)
	}

	rule2ID := "00000000-0000-0000-0000-000000000005"
	_, err = pool.Exec(context.Background(), `INSERT INTO mapping_rule (id, source_id, target_field_id, source_field, transformation_chain, validation_chain) VALUES ($1, $2, $3, 'raw_name', null, '[{"name": "not_null", "parameters": {}}]')`, rule2ID, srcID, tf2ID)
	if err != nil {
		t.Fatalf("insert rule 2: %v", err)
	}

	// Insert Raw data
	validPayload := []byte(`{"raw_email": " user@example.com ", "raw_name": "Alice"}`)
	invalidPayload := []byte(`{"raw_email": "invalid_email", "raw_name": "Bob"}`)

	var validID, invalidID string
	err = pool.QueryRow(context.Background(), `INSERT INTO raw_ingestion (topic, payload, nonce, dek_id) VALUES ('test_topic', $1, '', '') RETURNING id::text`, validPayload).Scan(&validID)
	if err != nil {
		t.Fatalf("insert valid raw: %v", err)
	}

	err = pool.QueryRow(context.Background(), `INSERT INTO raw_ingestion (topic, payload, nonce, dek_id) VALUES ('test_topic', $1, '', '') RETURNING id::text`, invalidPayload).Scan(&invalidID)
	if err != nil {
		t.Fatalf("insert invalid raw: %v", err)
	}

	// Run CLI Transformer
	dbCfgBytes, _ := json.Marshal(cfg)
	jobArgsBytes, _ := json.Marshal(map[string]interface{}{"workers": 2})

	cmd := exec.Command("go", "run", "../cmd/transformer/main.go", string(dbCfgBytes), string(jobArgsBytes))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cli run failed: %v\nOutput:\n%s", err, string(out))
	}
	t.Logf("CLI Output: %s", string(out))

	// Verify Target Table
	var count int
	var tName *string
	err = pool.QueryRow(context.Background(), `SELECT count(*), max("TargetName") FROM "test_topic"`).Scan(&count, &tName)
	if err != nil || count != 1 {
		t.Fatalf("expected 1 row in test_topic, got %d. Err: %v", count, err)
	}
	if tName == nil || *tName != "Alice" {
		t.Errorf("expected TargetName 'Alice', got '%v'", tName)
	}

	// Verify Statuses
	var vStatus, invStatus string
	pool.QueryRow(context.Background(), `SELECT status FROM raw_ingestion WHERE id = $1`, validID).Scan(&vStatus)
	pool.QueryRow(context.Background(), `SELECT status FROM raw_ingestion WHERE id = $1`, invalidID).Scan(&invStatus)

	if vStatus != "processed" {
		t.Errorf("expected valid record status 'processed', got '%s'", vStatus)
	}
	if invStatus != "failed_validation" {
		t.Errorf("expected invalid record status 'failed_validation', got '%s'", invStatus)
	}

	// Verify DLQ
	var errCount int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM transformation_errors WHERE raw_ingestion_id = $1`, invalidID).Scan(&errCount)
	if errCount != 1 {
		t.Errorf("expected 1 error in DLQ for invalid record, got %d", errCount)
	}

	// Test Retry Logic (--retry-failed)
	fixedPayload := []byte(`{"raw_email": "bob@example.com", "raw_name": "Bob"}`)
	pool.Exec(context.Background(), `UPDATE raw_ingestion SET payload = $1 WHERE id = $2`, fixedPayload, invalidID)

	retryJobArgsBytes, _ := json.Marshal(map[string]interface{}{"retry_failed": true, "workers": 2})
	retryCmd := exec.Command("go", "run", "../cmd/transformer/main.go", string(dbCfgBytes), string(retryJobArgsBytes))
	outRetry, errRetry := retryCmd.CombinedOutput()
	if errRetry != nil {
		t.Fatalf("cli retry run failed: %v\nOutput:\n%s", errRetry, string(outRetry))
	}
	t.Logf("CLI Retry Output: %s", string(outRetry))

	// Check Bob is now processed
	pool.QueryRow(context.Background(), `SELECT status FROM raw_ingestion WHERE id = $1`, invalidID).Scan(&invStatus)
	if invStatus != "processed" {
		t.Errorf("expected Bob record status 'processed' after retry, got '%s'", invStatus)
	}
}
