# MitM Transformation Layer (Rust)

Linux-targeted Rust replacement for `mitm_transformation`. It preserves the existing scheduler IPC protocol, PostgreSQL schema, job-argument JSON, transformation and validation rule names, AES-256-GCM envelope encryption, and DLQ behavior.

## Linux build

Install the stable Rust toolchain on the Linux build host, then build the optimized, dynamically independent binary:

```bash
cargo build --release
install -Dm755 target/release/mitm-transformation bin/mitm-transformer
```

The binary accepts the same optional job JSON as the Go implementation:

```bash
./bin/mitm-transformer '{"workers":5,"batch_size":500,"topic":"employee.data","required_sources":["ORA_EMPLOYEE"],"source_name":"TRANSFORMATION"}'
```

Database credentials are read first from `MITM_DB_CONFIG_JSON` (`{"db": {...}}`) and otherwise from `MITM_DB_HOST`, `MITM_DB_PORT`, `MITM_DB_USER`, `MITM_DB_PASSWORD`, `MITM_DB_NAME`, and `MITM_DB_SSLMODE`. Under the scheduler, `RUN_ID` and `SCHEDULER_SOCKET_PATH` activate the existing Unix-domain-socket credential and audit/status protocol.

`MASTER_KEY` may be Base64-encoded or literal text. It is normalized to 32 bytes for compatibility with the Go implementation.

## Verification

On a Linux host with Rust installed:

```bash
cargo fmt --check
cargo test
cargo clippy --all-targets -- -D warnings
```
