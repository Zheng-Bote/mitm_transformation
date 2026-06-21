# Changelog

All notable changes to the Transformation Layer will be documented in this file.

## [v0.7.0] - 2026-06-21
### Added
- **Stateful Aggregation**: Transformed the mapping layer from a 1:1 forwarder into an N:1 stateful aggregator. 
- **Topic Dependencies**: The Transformer now utilizes the `topic_dependencies` table to verify if all required source fragments for a given `correlation_id` are present in `raw_ingestion` before generating a "Golden Record".

### Changed
- **Correlation ID Engine**: Replaced legacy processing logic based on `raw_ingestion_id` with a `GROUP BY correlation_id` architecture.
- **Transformation Errors**: Updated the `transformation_errors` table to reference `correlation_id` instead of `raw_ingestion_id` for accurate Dead Letter Queue tracking.

## [v0.6.0] - 2026-06-15
### Added
- **Validators**: Added `min_length` and `max_length` validators to the transformation engine's validation library.
- **Centralized App Info & IPC**: Added `appName` and `version` globally. The component now broadcasts its name and version via IPC when starting.

## [v0.5.0] - 2026-06-10
### Added
- **Target Fragments Table**: Replaced dynamic target tables with a unified `target_fragments` table to store transformed payload JSON.

### Fixed
- **Case Sensitivity**: Fixed an issue where payload JSON key mapping failed because source systems provided differently cased fields than expected by the mapping rules. Mapping lookups are now case-insensitive.

## [v0.4.0] - 2026-06-09
### Fixed
- **Envelope Encryption**: Fixed a critical bug where `raw_ingestion.payload` was not decrypted prior to `json.Unmarshal`. The Transformer worker now retrieves the `wrapped_key` via a SQL JOIN and uses `EnvelopeDecrypt` to securely parse incoming fragments.

## [v0.2.0] - 2026-06-06
### Changed
- Refactored database initialization to read from `MITM_DB_*` environment variables instead of `os.Args[1]` to match the updated scheduler convention.
- Moved Job Arguments JSON from `os.Args[2]` to `os.Args[1]`.

## [v0.1.0] - 2026-06-06
### Added
- **Repositories**: `MappingRepo` for rule caching, `IngestionRepo` for fetching/claiming raw records via `FOR UPDATE SKIP LOCKED`, and `TargetRepo` for transacting target writes and tracking errors.
- **Pipeline Engine**: Core `PipelineEngine` to orchestrate mapping, transformation, validation, and encryption rules per source configuration.
- **Transformation Library**: Added functions `trim_whitespace`, `to_upper`, `to_lower`, `default_value`, `regex_replace`, `parse_date`, `string_split`, `cast_type`.
- **Validation Library**: Added functions `not_null`, `regex_match`, `range_check`, `email`, `in_list`.
- **Envelope Encryption**: `crypto/aes.go` for AES-256-GCM encryption of sensitive target fields.
- **CLI Batch-Job**: Go-based entrypoint `cmd/transformer/main.go` that acts as a worker-pool to consume the queue and exits gracefully.
- **DLQ Reprocessing**: Added `--retry-failed` flag to the CLI tool for re-processing failed validations.
- **E2E Tests**: Comprehensive end-to-end testing against the PostgreSQL instance confirming table inserts and DLQ handling.
