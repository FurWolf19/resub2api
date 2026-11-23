//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestUpdateQuotaMultiplierRevokedReturnsNotFound(t *testing.T) {
	svc := newQuotaTestService(&quotaTestRepo{})
	err := svc.UpdateQuotaMultiplier(context.Background(), 99, 0.5)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestExtendRevokedReturnsNotFound(t *testing.T) {
	svc := newQuotaTestService(&quotaTestRepo{})
	_, err := svc.ExtendSubscription(context.Background(), 99, 7)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestAdminResetQuotaRevokedReturnsNotFound(t *testing.T) {
	svc := newQuotaTestService(&quotaTestRepo{})
	_, err := svc.AdminResetQuotaWithFiveHour(context.Background(), 99, false, false, false, true)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestSubscriptionNotFoundErrorCode(t *testing.T) {
	statusCode, body := infraerrors.ToHTTP(ErrSubscriptionNotFound)
	require.Equal(t, http.StatusNotFound, statusCode)
	require.Equal(t, "SUBSCRIPTION_NOT_FOUND", body.Reason)
	require.True(t, errors.Is(ErrSubscriptionNotFound, ErrSubscriptionNotFound))
}
