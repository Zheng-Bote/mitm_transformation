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
	"log"

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

// WriteTargetAndComplete commits the final state for a processed aggregated fragment.
func (r *TargetRepo) WriteTargetAndComplete(ctx context.Context, correlationID string, topic string, data map[string]interface{}, errors []TransformationError, logAudit func(string)) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if len(errors) > 0 {
		for _, pe := range errors {
			msg := fmt.Sprintf("Validation Error for correlation %s on field %s: %s (Rule: %s)", correlationID, pe.FailedField, pe.ErrorMessage, pe.RuleName)
			log.Println(msg)
			// wasted due to logging to transformation_errors
			// if logAudit != nil {
			// 	logAudit(msg)
			// }
			_, err := tx.Exec(ctx, `INSERT INTO transformation_errors (correlation_id, failed_field, rule_name, error_message) VALUES ($1, $2, $3, $4)`, correlationID, pe.FailedField, pe.RuleName, pe.ErrorMessage)
			if err != nil {
				return fmt.Errorf("failed to insert into transformation_errors: %w", err)
			}
		}

		//if logAudit != nil {
		//	msg := fmt.Sprintf("%d validation errors logged for topic %s", len(errors), topic)
		//	logAudit(msg)
		//}

		_, err = tx.Exec(ctx, `UPDATE raw_ingestion SET status = 'failed_validation', processed_at = NOW() WHERE correlation_id = $1`, correlationID)
		if err != nil {
			return fmt.Errorf("failed to update raw_ingestion status: %w", err)
		}
	} else {
		if len(data) > 0 {
			query := `INSERT INTO target_fragments (correlation_id, topic, payload_jsonb, delivery_status) VALUES ($1, $2, $3, 'PENDING')`
			_, err = tx.Exec(ctx, query, correlationID, topic, data)
			if err != nil {
				return fmt.Errorf("failed to insert into target_fragments: %w", err)
			}
		}

		_, err = tx.Exec(ctx, `UPDATE raw_ingestion SET status = 'processed', processed_at = NOW() WHERE correlation_id = $1`, correlationID)
		if err != nil {
			return fmt.Errorf("failed to update raw_ingestion status: %w", err)
		}
	}

	return tx.Commit(ctx)
}
