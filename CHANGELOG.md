# Changelog

All notable changes to the Transformation Layer will be documented in this file.

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
