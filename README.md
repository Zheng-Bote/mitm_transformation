# MitM Transformation Layer (CLI Batch Job)

This directory contains the Go implementation of the MitM Transformation Layer, built as a robust CLI application.

## Overview

The Transformation Layer pulls data from the `raw_ingestion` table, applies mapping rules (transformation and validation pipelines dynamically sourced from PostgreSQL), encrypts sensitive target fields using AES-256 GCM, and writes the results to normalized destination tables. If validation fails, the payload is pushed to a Dead Letter Queue (DLQ) in the `transformation_errors` table.

This module is designed as a **Scheduled Batch Job** rather than a continuous daemon. It processes all pending records concurrently via a Go Worker Pool and terminates successfully when the queue is empty.

## Building

To build the executable, run:
```bash
go build -o bin/mitm-transformer ./cmd/transformer/main.go
```

## Usage

The batch job expects database credentials via environment variables and optional job configuration as a JSON argument (`os.Args[1]`), exactly as passed by the `mitm_scheduler`.

```bash
# 1. DB Config Environment Variables
export MITM_DB_HOST="192.168.0.31"
export MITM_DB_PORT="6543"
export MITM_DB_USER="mitm_user"
export MITM_DB_PASSWORD="..."
export MITM_DB_NAME="mitm"

# 2. Optional Job Args JSON
ARGS_JSON='{"workers": 5, "batch_size": 500, "retry_failed": false}'

./bin/mitm-transformer "$ARGS_JSON"
```

### Parameters
- `MITM_DB_*` (**Required**): Database connection parameters provided via Environment variables. (Alternatively `MITM_DB_CONFIG_JSON` can be used).
- `os.Args[1]` (**Optional**): Job configuration JSON object. Supported properties:
  - `workers`: Number of concurrent goroutines (default: 5).
  - `batch_size`: Number of records to claim atomically per cycle (default: 500).
  - `retry_failed`: Re-process records currently in `failed_validation` state instead of processing `pending` records.

## Dead Letter Queue (DLQ) & Reprocessing

If a transformation or validation fails:
1. An entry is created in `transformation_errors`.
2. The `status` of the `raw_ingestion` record is changed to `failed_validation`.

After correcting the root cause (e.g., updating a regex rule in `mapping_rule`), you can reprocess the failed records by setting `"retry_failed": true` in the job configuration JSON:

```bash
RETRY_ARGS='{"workers": 5, "batch_size": 500, "retry_failed": true}'
./bin/mitm-transformer "$RETRY_ARGS"
```
