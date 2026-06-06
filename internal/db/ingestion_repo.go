/**
 * SPDX-FileComment: Transformation Layer Database Migrations
 * SPDX-FileType: SOURCE
 * SPDX-FileContributor: ZHENG Robert
 * SPDX-FileCopyrightText: 2026 ZHENG Robert
 * SPDX-License-Identifier: Apache-2.0
 *
 * @file ingestion_repo.go
 * @brief Repository for interacting with raw ingestion data.
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

// RawFragment represents a single raw ingested record pending transformation.
type RawFragment struct {
	ID      string
	Topic   string
	Payload []byte
	Nonce   string
	DekID   string
}

// IngestionRepo handles fetching and updating raw ingestion fragments.
type IngestionRepo struct {
	pool *pgxpool.Pool
}

// NewIngestionRepo creates a new IngestionRepo instance.
func NewIngestionRepo(pool *pgxpool.Pool) *IngestionRepo {
	return &IngestionRepo{pool: pool}
}

// ClaimPendingFragments atomically retrieves a batch of pending records and sets their status to 'processing'.
// This avoids locking the rows across long transactions and allows multiple workers to safely process batches.
func (r *IngestionRepo) ClaimPendingFragments(ctx context.Context, limit int, retryFailed bool) ([]RawFragment, error) {
	statusFilter := "pending"
	if retryFailed {
		statusFilter = "failed_validation"
	}

	query := fmt.Sprintf(`
		UPDATE raw_ingestion 
		SET status = 'processing' 
		WHERE id IN (
			SELECT id FROM raw_ingestion 
			WHERE status = '%s' 
			ORDER BY created_at ASC 
			LIMIT $1 
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id::text, topic, payload, nonce, dek_id
	`, statusFilter)

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fragments []RawFragment
	for rows.Next() {
		var f RawFragment
		if err := rows.Scan(&f.ID, &f.Topic, &f.Payload, &f.Nonce, &f.DekID); err != nil {
			return nil, err
		}
		fragments = append(fragments, f)
	}
	return fragments, rows.Err()
}
