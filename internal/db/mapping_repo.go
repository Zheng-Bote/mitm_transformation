/**
 * SPDX-FileComment: Transformation Layer Database Migrations
 * SPDX-FileType: SOURCE
 * SPDX-FileContributor: ZHENG Robert
 * SPDX-FileCopyrightText: 2026 ZHENG Robert
 * SPDX-License-Identifier: Apache-2.0
 *
 * @file mapping_repo.go
 * @brief Repository for fetching and caching mapping rules.
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
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MappingSource represents a raw data source configuration.
type MappingSource struct {
	ID      string
	Name    string
	Type    string
	Topic   string
	Version int
}

// MappingTargetField represents a target field schema configuration.
type MappingTargetField struct {
	ID         string
	Topic      string
	FieldName  string
	DataType   string
	IsRequired bool
	Encrypted  bool
	Version    int
}

// RuleStep represents a single step in a transformation or validation chain.
type RuleStep struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}

// MappingRule represents a single rule to map and transform a source field to a target field.
type MappingRule struct {
	ID                    string
	SourceID              string
	TargetFieldID         string
	SourceField           string
	Priority              int
	TransformationChain   json.RawMessage
	ValidationChain       json.RawMessage
	ParsedTransformations []RuleStep
	ParsedValidations     []RuleStep
	Version               int
}

// RuleSet holds the in-memory cached configuration of sources, targets, and rules.
type RuleSet struct {
	Sources      map[string]MappingSource
	TargetFields map[string]MappingTargetField
	Rules        []MappingRule
}

// MappingRepo handles fetching configuration mapping rules from PostgreSQL.
type MappingRepo struct {
	pool    *pgxpool.Pool
	mu      sync.RWMutex
	ruleSet *RuleSet
}

// NewMappingRepo creates a new repository instance.
func NewMappingRepo(pool *pgxpool.Pool) *MappingRepo {
	return &MappingRepo{
		pool: pool,
	}
}

// LoadAndCache reads all configuration from the database and caches it in memory.
// This should be called once at the start of the batch job to minimize database roundtrips.
func (r *MappingRepo) LoadAndCache(ctx context.Context) error {
	sources, err := r.fetchSources(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch sources: %w", err)
	}

	targets, err := r.fetchTargets(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch targets: %w", err)
	}

	rules, err := r.fetchRules(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch rules: %w", err)
	}

	r.mu.Lock()
	r.ruleSet = &RuleSet{
		Sources:      sources,
		TargetFields: targets,
		Rules:        rules,
	}
	r.mu.Unlock()

	return nil
}

// GetCachedRules returns the current in-memory ruleset.
func (r *MappingRepo) GetCachedRules() *RuleSet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ruleSet
}

func (r *MappingRepo) fetchSources(ctx context.Context) (map[string]MappingSource, error) {
	rows, err := r.pool.Query(ctx, "SELECT id::text, name, type, topic, version FROM mapping_source")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := make(map[string]MappingSource)
	for rows.Next() {
		var s MappingSource
		if err := rows.Scan(&s.ID, &s.Name, &s.Type, &s.Topic, &s.Version); err != nil {
			return nil, err
		}
		sources[s.ID] = s
	}
	return sources, rows.Err()
}

func (r *MappingRepo) fetchTargets(ctx context.Context) (map[string]MappingTargetField, error) {
	rows, err := r.pool.Query(ctx, "SELECT id::text, topic, field_name, data_type, is_required, encrypted, version FROM mapping_target_field")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make(map[string]MappingTargetField)
	for rows.Next() {
		var t MappingTargetField
		if err := rows.Scan(&t.ID, &t.Topic, &t.FieldName, &t.DataType, &t.IsRequired, &t.Encrypted, &t.Version); err != nil {
			return nil, err
		}
		targets[t.ID] = t
	}
	return targets, rows.Err()
}

func (r *MappingRepo) fetchRules(ctx context.Context) ([]MappingRule, error) {
	rows, err := r.pool.Query(ctx, "SELECT id::text, source_id::text, target_field_id::text, source_field, priority, transformation_chain, validation_chain, version FROM mapping_rule")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []MappingRule
	for rows.Next() {
		var rule MappingRule
		if err := rows.Scan(&rule.ID, &rule.SourceID, &rule.TargetFieldID, &rule.SourceField, &rule.Priority, &rule.TransformationChain, &rule.ValidationChain, &rule.Version); err != nil {
			return nil, err
		}
		
		if len(rule.TransformationChain) > 0 && string(rule.TransformationChain) != "null" {
			_ = json.Unmarshal(rule.TransformationChain, &rule.ParsedTransformations)
		}
		if len(rule.ValidationChain) > 0 && string(rule.ValidationChain) != "null" {
			_ = json.Unmarshal(rule.ValidationChain, &rule.ParsedValidations)
		}
		
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}
