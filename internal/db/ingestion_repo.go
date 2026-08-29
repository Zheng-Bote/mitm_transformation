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

// RawFragment represents a single raw ingested record.
type RawFragment struct {
	ID         string
	Topic      string
	Payload    []byte
	Nonce      []byte
	DekID      string
	WrappedKey []byte
}

// AggregatedFragment represents a set of fragments belonging to the same correlation ID.
type AggregatedFragment struct {
	CorrelationID string
	Topic         string
	Fragments     []RawFragment
}

// IngestionRepo handles fetching and updating raw ingestion fragments.
type IngestionRepo struct {
	pool *pgxpool.Pool
}

// NewIngestionRepo creates a new IngestionRepo instance.
func NewIngestionRepo(pool *pgxpool.Pool) *IngestionRepo {
	return &IngestionRepo{pool: pool}
}

// ClaimAggregatedFragments retrieves correlation IDs that have all required sources and claims their fragments.
func (r *IngestionRepo) ClaimAggregatedFragments(ctx context.Context, topic string, requiredSources []string, limit int) ([]AggregatedFragment, error) {
	if len(requiredSources) == 0 {
		return nil, fmt.Errorf("requiredSources cannot be empty")
	}

	// 1. Find Correlation IDs that have all required sources
	queryFind := `
		SELECT correlation_id
		FROM raw_ingestion
		WHERE topic = $1 AND status = 'pending' AND correlation_id IS NOT NULL
		GROUP BY correlation_id
		HAVING COUNT(DISTINCT source_system) >= $2
		LIMIT $3
	`
	rows, err := r.pool.Query(ctx, queryFind, topic, len(requiredSources), limit)
	if err != nil {
		return nil, err
	}

	var correlationIDs []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			rows.Close()
			return nil, err
		}
		correlationIDs = append(correlationIDs, cid)
	}
	rows.Close()

	if len(correlationIDs) == 0 {
		return nil, nil // Nothing ready to be aggregated
	}

	// 2. Claim the actual fragments
	queryClaim := `
		UPDATE raw_ingestion
		SET status = 'processing', processed_at = NOW()
		FROM storage_keys
		WHERE raw_ingestion.correlation_id = ANY($1) 
		  AND raw_ingestion.topic = $2 
		  AND raw_ingestion.status = 'pending'
		  AND raw_ingestion.dek_id = storage_keys.id
		RETURNING raw_ingestion.id::text, raw_ingestion.topic, raw_ingestion.correlation_id::text, raw_ingestion.payload, raw_ingestion.nonce, raw_ingestion.dek_id::text, storage_keys.wrapped_key
	`
	claimRows, err := r.pool.Query(ctx, queryClaim, correlationIDs, topic)
	if err != nil {
		return nil, err
	}
	defer claimRows.Close()

	aggMap := make(map[string]*AggregatedFragment)

	for claimRows.Next() {
		var f RawFragment
		var cid string
		if err := claimRows.Scan(&f.ID, &f.Topic, &cid, &f.Payload, &f.Nonce, &f.DekID, &f.WrappedKey); err != nil {
			return nil, err
		}

		if _, exists := aggMap[cid]; !exists {
			aggMap[cid] = &AggregatedFragment{
				CorrelationID: cid,
				Topic:         topic,
				Fragments:     []RawFragment{},
			}
		}
		aggMap[cid].Fragments = append(aggMap[cid].Fragments, f)
	}

	var results []AggregatedFragment
	for _, agg := range aggMap {
		results = append(results, *agg)
	}

	return results, claimRows.Err()
}
