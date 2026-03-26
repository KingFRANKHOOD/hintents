// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

const maxPrintedLedgerDiffs = 20

type ledgerChangeKind string

const (
	ledgerChangeCreated ledgerChangeKind = "created"
	ledgerChangeUpdated ledgerChangeKind = "updated"
	ledgerChangeRemoved ledgerChangeKind = "removed"
)

type ledgerEntryDiff struct {
	Key    string
	Kind   ledgerChangeKind
	Before *xdr.LedgerEntry
	After  *xdr.LedgerEntry
}

type ledgerDiffSummary struct {
	Created int
	Updated int
	Removed int
	Diffs   []ledgerEntryDiff
}

type stateTransition struct {
	before *xdr.LedgerEntry
	after  *xdr.LedgerEntry
}

func summarizeLedgerEntryDiffs(resultMetaXDR string, beforeState map[string]string) (*ledgerDiffSummary, error) {
	beforeEntries, err := decodeLedgerEntryState(beforeState)
	if err != nil {
		return nil, err
	}

	meta, err := decodeResultMeta(resultMetaXDR)
	if err != nil {
		return nil, err
	}

	current := cloneLedgerEntryMap(beforeEntries)
	transitions := make(map[string]*stateTransition)

	applyMutatingChanges := func(changes xdr.LedgerEntryChanges) {
		for _, change := range changes {
			switch change.Type {
			case xdr.LedgerEntryChangeTypeLedgerEntryCreated:
				if change.Created == nil {
					continue
				}
				key, err := encodeLedgerKeyFromEntry(*change.Created)
				if err != nil {
					continue
				}
				recordTransition(transitions, current, key)
				current[key] = cloneLedgerEntry(*change.Created)
				transitions[key].after = cloneLedgerEntry(*change.Created)

			case xdr.LedgerEntryChangeTypeLedgerEntryUpdated:
				if change.Updated == nil {
					continue
				}
				key, err := encodeLedgerKeyFromEntry(*change.Updated)
				if err != nil {
					continue
				}
				recordTransition(transitions, current, key)
				current[key] = cloneLedgerEntry(*change.Updated)
				transitions[key].after = cloneLedgerEntry(*change.Updated)

			case xdr.LedgerEntryChangeTypeLedgerEntryRemoved:
				if change.Removed == nil {
					continue
				}
				key, err := encodeLedgerKey(*change.Removed)
				if err != nil {
					continue
				}
				recordTransition(transitions, current, key)
				delete(current, key)
				transitions[key].after = nil

			case xdr.LedgerEntryChangeTypeLedgerEntryState:
				// State snapshots are baseline context, not mutations. We keep them
				// only to improve follow-up update/remove transitions if needed.
				if change.State == nil {
					continue
				}
				key, err := encodeLedgerKeyFromEntry(*change.State)
				if err != nil {
					continue
				}
				if _, exists := current[key]; !exists {
					current[key] = cloneLedgerEntry(*change.State)
				}
			}
		}
	}

	applyMutatingChanges(meta.FeeProcessing)

	switch meta.TxApplyProcessing.V {
	case 0:
		if meta.TxApplyProcessing.Operations != nil {
			for _, op := range *meta.TxApplyProcessing.Operations {
				applyMutatingChanges(op.Changes)
			}
		}
	case 1:
		if v1 := meta.TxApplyProcessing.V1; v1 != nil {
			applyMutatingChanges(v1.TxChanges)
			for _, op := range v1.Operations {
				applyMutatingChanges(op.Changes)
			}
		}
	case 2:
		if v2 := meta.TxApplyProcessing.V2; v2 != nil {
			applyMutatingChanges(v2.TxChangesBefore)
			applyMutatingChanges(v2.TxChangesAfter)
			for _, op := range v2.Operations {
				applyMutatingChanges(op.Changes)
			}
		}
	case 3:
		if v3 := meta.TxApplyProcessing.V3; v3 != nil {
			applyMutatingChanges(v3.TxChangesBefore)
			applyMutatingChanges(v3.TxChangesAfter)
			for _, op := range v3.Operations {
				applyMutatingChanges(op.Changes)
			}
		}
	}

	keys := make([]string, 0, len(transitions))
	for key := range transitions {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	summary := &ledgerDiffSummary{}
	for _, key := range keys {
		tr := transitions[key]
		kind, ok := classifyTransition(tr.before, tr.after)
		if !ok {
			continue
		}

		diff := ledgerEntryDiff{
			Key:    key,
			Kind:   kind,
			Before: cloneLedgerEntryPtr(tr.before),
			After:  cloneLedgerEntryPtr(tr.after),
		}
		summary.Diffs = append(summary.Diffs, diff)

		switch kind {
		case ledgerChangeCreated:
			summary.Created++
		case ledgerChangeUpdated:
			summary.Updated++
		case ledgerChangeRemoved:
			summary.Removed++
		}
	}

	return summary, nil
}

func renderLedgerDiffSummary(summary *ledgerDiffSummary) string {
	if summary == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n=== Ledger State Diff (re-simulated) ===\n")
	total := summary.Created + summary.Updated + summary.Removed
	fmt.Fprintf(&b, "Changed entries: %d (created=%d, updated=%d, removed=%d)\n", total, summary.Created, summary.Updated, summary.Removed)

	if total == 0 {
		b.WriteString("No ledger entry changes detected.\n")
		return b.String()
	}

	limit := len(summary.Diffs)
	if limit > maxPrintedLedgerDiffs {
		limit = maxPrintedLedgerDiffs
	}

	for i := 0; i < limit; i++ {
		d := summary.Diffs[i]
		fmt.Fprintf(&b, "[%s] key=%s\n", strings.ToUpper(string(d.Kind)), shortLedgerKey(d.Key))
		switch d.Kind {
		case ledgerChangeCreated:
			fmt.Fprintf(&b, "  after:  %s\n", ledgerEntryPreview(d.After))
		case ledgerChangeUpdated:
			fmt.Fprintf(&b, "  before: %s\n", ledgerEntryPreview(d.Before))
			fmt.Fprintf(&b, "  after:  %s\n", ledgerEntryPreview(d.After))
		case ledgerChangeRemoved:
			fmt.Fprintf(&b, "  before: %s\n", ledgerEntryPreview(d.Before))
		}
	}

	if len(summary.Diffs) > limit {
		fmt.Fprintf(&b, "... and %d more changed entries\n", len(summary.Diffs)-limit)
	}

	return b.String()
}

func decodeLedgerEntryState(entries map[string]string) (map[string]xdr.LedgerEntry, error) {
	decoded := make(map[string]xdr.LedgerEntry, len(entries))
	for key, value := range entries {
		entry, err := decodeLedgerEntry(value)
		if err != nil {
			return nil, fmt.Errorf("decode ledger entry for key %s: %w", shortLedgerKey(key), err)
		}
		decoded[key] = entry
	}
	return decoded, nil
}

func decodeResultMeta(resultMetaXDR string) (*xdr.TransactionResultMeta, error) {
	b, err := base64.StdEncoding.DecodeString(resultMetaXDR)
	if err != nil {
		return nil, fmt.Errorf("decode result meta base64: %w", err)
	}

	var meta xdr.TransactionResultMeta
	if err := xdr.SafeUnmarshal(b, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal result meta xdr: %w", err)
	}

	return &meta, nil
}

func decodeLedgerEntry(entryXDR string) (xdr.LedgerEntry, error) {
	b, err := base64.StdEncoding.DecodeString(entryXDR)
	if err != nil {
		return xdr.LedgerEntry{}, fmt.Errorf("decode entry base64: %w", err)
	}

	var entry xdr.LedgerEntry
	if err := xdr.SafeUnmarshal(b, &entry); err != nil {
		return xdr.LedgerEntry{}, fmt.Errorf("unmarshal entry xdr: %w", err)
	}

	return entry, nil
}

func encodeLedgerKey(key xdr.LedgerKey) (string, error) {
	b, err := key.MarshalBinary()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func encodeLedgerKeyFromEntry(entry xdr.LedgerEntry) (string, error) {
	key, err := entry.LedgerKey()
	if err != nil {
		return "", err
	}
	return encodeLedgerKey(key)
}

func recordTransition(transitions map[string]*stateTransition, current map[string]xdr.LedgerEntry, key string) {
	if _, exists := transitions[key]; exists {
		return
	}

	var before *xdr.LedgerEntry
	if entry, ok := current[key]; ok {
		copy := cloneLedgerEntry(entry)
		before = &copy
	}

	transitions[key] = &stateTransition{before: before}
}

func classifyTransition(before, after *xdr.LedgerEntry) (ledgerChangeKind, bool) {
	switch {
	case before == nil && after == nil:
		return "", false
	case before == nil && after != nil:
		return ledgerChangeCreated, true
	case before != nil && after == nil:
		return ledgerChangeRemoved, true
	default:
		if ledgerEntriesEqual(*before, *after) {
			return "", false
		}
		return ledgerChangeUpdated, true
	}
}

func ledgerEntriesEqual(a, b xdr.LedgerEntry) bool {
	ab, err := a.MarshalBinary()
	if err != nil {
		return false
	}
	bb, err := b.MarshalBinary()
	if err != nil {
		return false
	}
	if len(ab) != len(bb) {
		return false
	}
	for i := range ab {
		if ab[i] != bb[i] {
			return false
		}
	}
	return true
}

func cloneLedgerEntryMap(src map[string]xdr.LedgerEntry) map[string]xdr.LedgerEntry {
	dst := make(map[string]xdr.LedgerEntry, len(src))
	for key, value := range src {
		dst[key] = cloneLedgerEntry(value)
	}
	return dst
}

func cloneLedgerEntry(entry xdr.LedgerEntry) xdr.LedgerEntry {
	b, err := entry.MarshalBinary()
	if err != nil {
		return entry
	}
	var cloned xdr.LedgerEntry
	if err := xdr.SafeUnmarshal(b, &cloned); err != nil {
		return entry
	}
	return cloned
}

func cloneLedgerEntryPtr(entry *xdr.LedgerEntry) *xdr.LedgerEntry {
	if entry == nil {
		return nil
	}
	cloned := cloneLedgerEntry(*entry)
	return &cloned
}

func shortLedgerKey(key string) string {
	if len(key) <= 18 {
		return key
	}
	return key[:8] + "..." + key[len(key)-8:]
}

func ledgerEntryPreview(entry *xdr.LedgerEntry) string {
	if entry == nil {
		return "<none>"
	}
	b, err := entry.MarshalBinary()
	if err != nil {
		return fmt.Sprintf("type=%s", entry.Data.Type)
	}
	xdrB64 := base64.StdEncoding.EncodeToString(b)
	if len(xdrB64) > 32 {
		xdrB64 = xdrB64[:32] + "..."
	}
	return fmt.Sprintf("type=%s seq=%d xdr=%s", entry.Data.Type, entry.LastModifiedLedgerSeq, xdrB64)
}