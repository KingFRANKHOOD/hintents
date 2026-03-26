// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/base64"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizeLedgerEntryDiffs(t *testing.T) {
	acct1 := xdr.MustAddress("GCRRSYF5JBFPXHN5DCG65A4J3MUYE53QMQ4XMXZ3CNKWFJIJJTGMH6MZ")
	acct2 := xdr.MustAddress("GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H")
	acct3 := xdr.MustAddress("GDX2V6GLB54W2U4WZ7AFCL5J2R2F6M7XSWC6IHTPP6W6V4WTK6QJ2C6K")

	updatedBefore := makeAccountEntry(acct1, 100, 10)
	updatedAfter := makeAccountEntry(acct1, 150, 11)
	removedBefore := makeAccountEntry(acct2, 200, 20)
	createdAfter := makeAccountEntry(acct3, 300, 30)

	beforeState := map[string]string{}
	updatedKey := mustEncodeLedgerKeyFromEntry(t, updatedBefore)
	removedKey := mustEncodeLedgerKeyFromEntry(t, removedBefore)
	beforeState[updatedKey] = mustEncodeLedgerEntry(t, updatedBefore)
	beforeState[removedKey] = mustEncodeLedgerEntry(t, removedBefore)

	removedLedgerKey, err := removedBefore.LedgerKey()
	require.NoError(t, err)

	changes := xdr.LedgerEntryChanges{
		{
			Type:    xdr.LedgerEntryChangeTypeLedgerEntryUpdated,
			Updated: &updatedAfter,
		},
		{
			Type:    xdr.LedgerEntryChangeTypeLedgerEntryRemoved,
			Removed: &removedLedgerKey,
		},
		{
			Type:    xdr.LedgerEntryChangeTypeLedgerEntryCreated,
			Created: &createdAfter,
		},
	}

	metaB64 := mustEncodeResultMetaWithChanges(t, changes)

	summary, err := summarizeLedgerEntryDiffs(metaB64, beforeState)
	require.NoError(t, err)
	require.NotNil(t, summary)

	assert.Equal(t, 1, summary.Created)
	assert.Equal(t, 1, summary.Updated)
	assert.Equal(t, 1, summary.Removed)
	assert.Len(t, summary.Diffs, 3)

	countsByKind := map[ledgerChangeKind]int{}
	for _, diff := range summary.Diffs {
		countsByKind[diff.Kind]++
	}

	assert.Equal(t, 1, countsByKind[ledgerChangeCreated])
	assert.Equal(t, 1, countsByKind[ledgerChangeUpdated])
	assert.Equal(t, 1, countsByKind[ledgerChangeRemoved])
}

func TestRenderLedgerDiffSummary(t *testing.T) {
	summary := &ledgerDiffSummary{
		Created: 1,
		Updated: 1,
		Removed: 1,
		Diffs: []ledgerEntryDiff{
			{Key: "AAAAAAAAAAAAAAAAAAAAAA==", Kind: ledgerChangeCreated, After: &xdr.LedgerEntry{Data: xdr.LedgerEntryData{Type: xdr.LedgerEntryTypeAccount}}},
			{Key: "BBBBBBBBBBBBBBBBBBBBBB==", Kind: ledgerChangeUpdated, Before: &xdr.LedgerEntry{Data: xdr.LedgerEntryData{Type: xdr.LedgerEntryTypeAccount}}, After: &xdr.LedgerEntry{Data: xdr.LedgerEntryData{Type: xdr.LedgerEntryTypeAccount}}},
			{Key: "CCCCCCCCCCCCCCCCCCCCCC==", Kind: ledgerChangeRemoved, Before: &xdr.LedgerEntry{Data: xdr.LedgerEntryData{Type: xdr.LedgerEntryTypeAccount}}},
		},
	}

	out := renderLedgerDiffSummary(summary)
	assert.Contains(t, out, "Ledger State Diff")
	assert.Contains(t, out, "Changed entries: 3")
	assert.Contains(t, out, "[CREATED]")
	assert.Contains(t, out, "[UPDATED]")
	assert.Contains(t, out, "[REMOVED]")
}

func makeAccountEntry(addr xdr.Address, balance xdr.Int64, seq uint32) xdr.LedgerEntry {
	return xdr.LedgerEntry{
		LastModifiedLedgerSeq: xdr.Uint32(seq),
		Data: xdr.LedgerEntryData{
			Type: xdr.LedgerEntryTypeAccount,
			Account: &xdr.AccountEntry{
				AccountId: addr.ToAccountId(),
				Balance:   balance,
			},
		},
	}
}

func mustEncodeLedgerEntry(t *testing.T, entry xdr.LedgerEntry) string {
	t.Helper()
	b, err := entry.MarshalBinary()
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

func mustEncodeLedgerKeyFromEntry(t *testing.T, entry xdr.LedgerEntry) string {
	t.Helper()
	key, err := entry.LedgerKey()
	require.NoError(t, err)
	b, err := key.MarshalBinary()
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

func mustEncodeResultMetaWithChanges(t *testing.T, changes xdr.LedgerEntryChanges) string {
	t.Helper()

	txMeta, err := xdr.NewTransactionMeta(1, xdr.TransactionMetaV1{
		TxChanges: changes,
		Operations: []xdr.OperationMeta{
			{Changes: changes},
		},
	})
	require.NoError(t, err)

	meta := xdr.TransactionResultMeta{
		FeeProcessing:     xdr.LedgerEntryChanges{},
		TxApplyProcessing: txMeta,
		Result: xdr.TransactionResultPair{
			TransactionHash: xdr.Hash{1, 2, 3},
			Result: xdr.TransactionResult{
				FeeCharged: 100,
				Result: xdr.TransactionResultResult{
					Code:    xdr.TransactionResultCodeTxSuccess,
					Results: &[]xdr.OperationResult{},
				},
				Ext: xdr.TransactionResultExt{V: 0},
			},
		},
	}

	b, err := meta.MarshalBinary()
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}