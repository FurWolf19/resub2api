//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func newProtocolCapabilitySQLMockRepo(t *testing.T) (*accountRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return &accountRepository{client: client}, mock
}

func TestMergeOpenAIProtocolCapabilitiesExtraPreservesLargeNumbers(t *testing.T) {
	const original = `{"big":12345678901234567890,"nested":{"n":9007199254740993},"openai_responses_supported":true}`
	extra := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal([]byte(original), &extra))
	supported := true

	payload, changed, err := mergeOpenAIProtocolCapabilitiesExtra(extra, []service.OpenAIProtocolCapabilityPatch{{
		Protocol:  openai_compat.ProtocolResponses,
		Supported: &supported,
		ProbedAt:  nowForProtocolCapabilityUnitTest(),
	}})
	require.NoError(t, err)
	require.True(t, changed)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, "12345678901234567890", string(decoded["big"]))
	require.Equal(t, `{"n":9007199254740993}`, string(decoded["nested"]))
	require.Contains(t, string(payload), `"openai_protocol_capabilities"`)
}

func TestMergeOpenAIProtocolCapabilitiesExtraSkipsUpdateWhenUnchanged(t *testing.T) {
	now := nowForProtocolCapabilityUnitTest()
	extra := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal([]byte(`{"custom":"x","openai_responses_supported":true}`), &extra))
	patches := []service.OpenAIProtocolCapabilityPatch{{Protocol: openai_compat.ProtocolResponses, ProbedAt: now}}
	// 先写一次，拿到完整快照。
	first, changed, err := mergeOpenAIProtocolCapabilitiesExtra(extra, patches)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, json.Unmarshal(first, &extra))

	_, changed, err = mergeOpenAIProtocolCapabilitiesExtra(extra, patches)
	require.NoError(t, err)
	require.False(t, changed, "相同 probed_at 的 touch 不应产生重复写")
}

func TestMergeOpenAIProtocolCapabilitiesExtraLegacyNullStaysUnknown(t *testing.T) {
	extra := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal([]byte(`{"openai_responses_supported":null,"custom":"x"}`), &extra))
	chatSupported := true

	payload, changed, err := mergeOpenAIProtocolCapabilitiesExtra(extra, []service.OpenAIProtocolCapabilityPatch{{
		Protocol:  openai_compat.ProtocolChatCompletions,
		Supported: &chatSupported,
		ProbedAt:  nowForProtocolCapabilityUnitTest(),
	}})
	require.NoError(t, err)
	require.True(t, changed)

	var rawDecoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &rawDecoded))
	require.Equal(t, "null", string(rawDecoded[openai_compat.ExtraKeyResponsesSupported]), "legacy null 不能被改写成 false")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	caps := openai_compat.ResolveProtocolCapabilities(decoded)
	require.False(t, caps.Responses.Known, "legacy null 必须保持 unknown，不能合成 responses 结论")
	require.True(t, caps.ChatCompletions.Known)
	require.True(t, caps.ChatCompletions.Supported)
	require.Equal(t, "x", decoded["custom"])
}

func TestMergeOpenAIProtocolCapabilitiesExtraLegacyValueSemantics(t *testing.T) {
	now := nowForProtocolCapabilityUnitTest()
	chatSupported := true
	tests := []struct {
		name          string
		legacy        string
		wantMissing   bool
		wantRaw       string
		wantKnown     bool
		wantSupported bool
	}{
		{name: "missing stays unknown", wantMissing: true},
		{name: "null stays unknown", legacy: "null", wantRaw: "null"},
		{name: "false preserved", legacy: "false", wantRaw: "false", wantKnown: true, wantSupported: false},
		{name: "true preserved", legacy: "true", wantRaw: "true", wantKnown: true, wantSupported: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extra := map[string]json.RawMessage{}
			if tt.legacy != "" {
				extra[openai_compat.ExtraKeyResponsesSupported] = json.RawMessage(tt.legacy)
			}
			payload, changed, err := mergeOpenAIProtocolCapabilitiesExtra(extra, []service.OpenAIProtocolCapabilityPatch{{
				Protocol:  openai_compat.ProtocolChatCompletions,
				Supported: &chatSupported,
				ProbedAt:  now,
			}})
			require.NoError(t, err)
			require.True(t, changed)

			var rawDecoded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(payload, &rawDecoded))
			if tt.wantMissing {
				require.NotContains(t, rawDecoded, openai_compat.ExtraKeyResponsesSupported)
			} else {
				require.Equal(t, tt.wantRaw, string(rawDecoded[openai_compat.ExtraKeyResponsesSupported]))
			}

			var decoded map[string]any
			require.NoError(t, json.Unmarshal(payload, &decoded))
			caps := openai_compat.ResolveProtocolCapabilities(decoded)
			require.Equal(t, tt.wantKnown, caps.Responses.Known)
			if tt.wantKnown {
				require.Equal(t, tt.wantSupported, caps.Responses.Supported)
			}
		})
	}
}

func nowForProtocolCapabilityUnitTest() time.Time {
	return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
}

func TestMergeOpenAIProtocolCapabilitiesLocksAndWrites(t *testing.T) {
	repo, mock := newProtocolCapabilitySQLMockRepo(t)
	ctx := context.Background()
	accountID := int64(42)
	supported := false

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(extra, '{}'::jsonb) FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE")).
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"extra"}).
			AddRow([]byte(`{"custom":"x","openai_responses_supported":true}`)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE accounts SET extra = $1::jsonb, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL")).
		WithArgs(sqlmock.AnyArg(), accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MergeOpenAIProtocolCapabilities(ctx, accountID, []service.OpenAIProtocolCapabilityPatch{{
		Protocol:  openai_compat.ProtocolResponses,
		Supported: &supported,
		ProbedAt:  nowForProtocolCapabilityUnitTest(),
	}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMergeOpenAIProtocolCapabilitiesNotFoundRollsBack(t *testing.T) {
	repo, mock := newProtocolCapabilitySQLMockRepo(t)
	ctx := context.Background()
	accountID := int64(43)
	supported := true

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(extra, '{}'::jsonb) FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE")).
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"extra"}))
	mock.ExpectRollback()

	err := repo.MergeOpenAIProtocolCapabilities(ctx, accountID, []service.OpenAIProtocolCapabilityPatch{{
		Protocol:  openai_compat.ProtocolResponses,
		Supported: &supported,
		ProbedAt:  nowForProtocolCapabilityUnitTest(),
	}})
	require.ErrorIs(t, err, service.ErrAccountNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMergeOpenAIProtocolCapabilitiesMergeErrorRollsBack(t *testing.T) {
	repo, mock := newProtocolCapabilitySQLMockRepo(t)
	ctx := context.Background()
	accountID := int64(44)
	supported := true

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(extra, '{}'::jsonb) FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE")).
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow([]byte(`{}`)))
	mock.ExpectRollback()

	err := repo.MergeOpenAIProtocolCapabilities(ctx, accountID, []service.OpenAIProtocolCapabilityPatch{{
		Protocol:  "bogus",
		Supported: &supported,
		ProbedAt:  nowForProtocolCapabilityUnitTest(),
	}})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMergeOpenAIProtocolCapabilitiesUsesExistingTransaction(t *testing.T) {
	repo, mock := newProtocolCapabilitySQLMockRepo(t)
	supported := false
	accountID := int64(45)

	mock.ExpectBegin()
	tx, err := repo.client.Tx(context.Background())
	require.NoError(t, err)
	ctx := dbent.NewTxContext(context.Background(), tx)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(extra, '{}'::jsonb) FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE")).
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow([]byte(`{}`)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE accounts SET extra = $1::jsonb, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL")).
		WithArgs(sqlmock.AnyArg(), accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.MergeOpenAIProtocolCapabilities(ctx, accountID, []service.OpenAIProtocolCapabilityPatch{{
		Protocol:  openai_compat.ProtocolResponses,
		Supported: &supported,
		ProbedAt:  nowForProtocolCapabilityUnitTest(),
	}}))
	mock.ExpectCommit()
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}
