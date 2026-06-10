/**
 * SPDX-FileComment: Transformation Layer Database Migrations
 * SPDX-FileType: SOURCE
 * SPDX-FileContributor: ZHENG Robert
 * SPDX-FileCopyrightText: 2026 ZHENG Robert
 * SPDX-License-Identifier: Apache-2.0
 *
 * @file target_repo.go
 * @brief Repository for writing transformed data and handling DLQ entries.
 * @version 1.0.0
 * @date 2026-06-06
 *
 * @author ZHENG Robert (robert@hase-zheng.net)
 * @copyright Copyright (c) 2026 ZHENG Robert
 * @license Apache-2.0
 */

package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TransformationError holds details for DLQ logging.
type TransformationError struct {
	FailedField  string
	RuleName     string
	ErrorMessage string
}

// TargetRepo handles writing successful transformations to target tables and logging errors.
type TargetRepo struct {
	pool *pgxpool.Pool
}

// NewTargetRepo creates a new target repository.
func NewTargetRepo(pool *pgxpool.Pool) *TargetRepo {
	return &TargetRepo{pool: pool}
}

// WriteTargetAndComplete commits the final state for a processed fragment.
// If there are errors, it writes to DLQ and updates status to failed_validation.
// If successful, it writes to the target table (topic) and updates status to processed.
func (r *TargetRepo) WriteTargetAndComplete(ctx context.Context, fragmentID string, topic string, data map[string]interface{}, errors []TransformationError) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if len(errors) > 0 {
		for _, pe := range errors {
			_, err = tx.Exec(ctx, `INSERT INTO transformation_errors (raw_ingestion_id, failed_field, rule_name, error_message) VALUES ($1, $2, $3, $4)`, fragmentID, pe.FailedField, pe.RuleName, pe.ErrorMessage)
			if err != nil {
				return fmt.Errorf("failed to insert transformation error: %w", err)
			}
		}
		_, err = tx.Exec(ctx, `UPDATE raw_ingestion SET status = 'failed_validation', processed_at = NOW() WHERE id = $1`, fragmentID)
		if err != nil {
			return fmt.Errorf("failed to update raw_ingestion status: %w", err)
		}
	} else {
		if len(data) > 0 {
			// Write JSON payload into generic target_fragments table
			query := `INSERT INTO target_fragments (raw_ingestion_id, topic, payload_jsonb, delivery_status) VALUES ($1, $2, $3, 'PENDING')`
			_, err = tx.Exec(ctx, query, fragmentID, topic, data)
			if err != nil {
				return fmt.Errorf("failed to insert into target_fragments: %w", err)
			}
		}

		_, err = tx.Exec(ctx, `UPDATE raw_ingestion SET status = 'processed', processed_at = NOW() WHERE id = $1`, fragmentID)
		if err != nil {
			return fmt.Errorf("failed to update raw_ingestion status: %w", err)
		}
	}

	return tx.Commit(ctx)
}
