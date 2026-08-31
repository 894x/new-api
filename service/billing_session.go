package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const billingRefundReconcileMaxAttempts = 3

var (
	ErrBillingSessionRequiresReconciliation      = errors.New("billing session requires manual reconciliation")
	ErrBillingRefundRequiresManualReconciliation = errors.New("billing refund requires manual reconciliation")
)

// ReconcilePendingBillingRefundOperation advances only refund steps whose
// durable state proves they are safe to execute. Ready actions are claimed as
// unknown before their external side effect starts. An unknown non-idempotent
// action is never repeated; only the subscription's request-id-idempotent
// initial refund may be retried from unknown.
func ReconcilePendingBillingRefundOperation(operationID string) (model.BillingRefundOperation, error) {
	return reconcilePendingBillingRefundOperation(operationID, true)
}

func reconcilePendingBillingRefundOperation(operationID string, allowSubscriptionUnknown bool) (model.BillingRefundOperation, error) {
	var operation model.BillingRefundOperation
	var lastErr error
	for attempt := 0; attempt < billingRefundReconcileMaxAttempts; attempt++ {
		operation, lastErr = reconcilePendingBillingRefundOperationOnce(operationID, allowSubscriptionUnknown)
		if lastErr == nil || errors.Is(lastErr, ErrBillingRefundRequiresManualReconciliation) {
			return operation, lastErr
		}
		if attempt < billingRefundReconcileMaxAttempts-1 {
			time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
		}
	}
	return operation, lastErr
}

func reconcilePendingBillingRefundOperationOnce(operationID string, allowSubscriptionUnknown bool) (model.BillingRefundOperation, error) {
	for step := 0; step < 8; step++ {
		operation, err := model.GetBillingRefundOperation(operationID)
		if err != nil {
			if errors.Is(err, model.ErrBillingRefundCorrupt) {
				return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, err)
			}
			return operation, err
		}
		if operation.Status == model.BillingRefundStatusApplied {
			return operation, nil
		}

		switch operation.PendingAction {
		case model.BillingRefundPendingActionFundingReady:
			runBillingBeforeActionClaimHook(operationID, model.BillingRefundPendingActionFundingReady)
			claim, claimErr := model.ClaimNextBillingRefundAction(operationID, model.BillingRefundPendingActionFundingReady)
			if claimErr != nil {
				if errors.Is(claimErr, model.ErrBillingRefundCorrupt) {
					return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, claimErr)
				}
				return operation, claimErr
			}
			if !claim.Claimed {
				continue
			}
			operation = claim.Operation
			var refundErr error
			switch operation.FundingSource {
			case BillingSourceWallet:
				refundErr = model.RefundUserQuotaForBilling(int(operation.FundingReferenceID), operation.FundingQuota)
			case BillingSourceSubscription:
				refundErr = model.RefundSubscriptionPreConsume(operation.RequestID)
			default:
				refundErr = model.ErrBillingRefundCorrupt
			}
			if refundErr != nil {
				if operation.FundingSource == BillingSourceSubscription {
					return operation, refundErr
				}
				return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, refundErr)
			}
			if confirmErr := model.ConfirmBillingRefundFunding(operationID); confirmErr != nil {
				if operation.FundingSource == BillingSourceSubscription {
					return operation, confirmErr
				}
				return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, confirmErr)
			}

		case model.BillingRefundPendingActionFundingUnknown:
			if !allowSubscriptionUnknown || operation.FundingSource != BillingSourceSubscription {
				return operation, ErrBillingRefundRequiresManualReconciliation
			}
			if refundErr := model.RefundSubscriptionPreConsume(operation.RequestID); refundErr != nil {
				return operation, refundErr
			}
			if confirmErr := model.ConfirmBillingRefundFunding(operationID); confirmErr != nil {
				return operation, confirmErr
			}

		case model.BillingRefundPendingActionSubscriptionExtraReady:
			runBillingBeforeActionClaimHook(operationID, model.BillingRefundPendingActionSubscriptionExtraReady)
			claim, claimErr := model.ClaimNextBillingRefundAction(operationID, model.BillingRefundPendingActionSubscriptionExtraReady)
			if claimErr != nil {
				if errors.Is(claimErr, model.ErrBillingRefundCorrupt) {
					return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, claimErr)
				}
				return operation, claimErr
			}
			if !claim.Claimed {
				continue
			}
			operation = claim.Operation
			if refundErr := model.PostConsumeUserSubscriptionDelta(int(operation.FundingReferenceID), -int64(operation.SubscriptionExtraQuota)); refundErr != nil {
				return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, refundErr)
			}
			if confirmErr := model.ConfirmBillingRefundSubscriptionExtra(operationID); confirmErr != nil {
				return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, confirmErr)
			}

		case model.BillingRefundPendingActionSubscriptionExtraUnknown,
			model.BillingRefundPendingActionTokenUnknown:
			return operation, ErrBillingRefundRequiresManualReconciliation

		case model.BillingRefundPendingActionTokenReady:
			token, tokenErr := model.GetTokenById(operation.TokenID)
			if tokenErr != nil {
				return operation, tokenErr
			}
			if token.UserId != operation.UserID {
				return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, model.ErrBillingRefundCorrupt)
			}
			runBillingBeforeActionClaimHook(operationID, model.BillingRefundPendingActionTokenReady)
			claim, claimErr := model.ClaimNextBillingRefundAction(operationID, model.BillingRefundPendingActionTokenReady)
			if claimErr != nil {
				if errors.Is(claimErr, model.ErrBillingRefundCorrupt) {
					return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, claimErr)
				}
				return operation, claimErr
			}
			if !claim.Claimed {
				continue
			}
			operation = claim.Operation
			if refundErr := model.RefundTokenQuotaForBilling(operation.TokenID, token.Key, operation.TokenQuota); refundErr != nil {
				return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, refundErr)
			}
			if confirmErr := model.ConfirmBillingRefundToken(operationID); confirmErr != nil {
				return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, confirmErr)
			}

		case model.BillingRefundPendingActionCommitAfterRefund:
			if commitErr := model.CommitBillingRefundOperation(operationID); commitErr != nil {
				return operation, commitErr
			}

		default:
			return operation, errors.Join(ErrBillingRefundRequiresManualReconciliation, model.ErrBillingRefundCorrupt)
		}
	}
	operation, err := model.GetBillingRefundOperation(operationID)
	if err != nil {
		return operation, err
	}
	if operation.Status != model.BillingRefundStatusApplied {
		return operation, ErrBillingRefundRequiresManualReconciliation
	}
	return operation, nil
}

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo        *relaycommon.RelayInfo
	funding          FundingSource
	preConsumedQuota int  // 实际预扣额度（信任用户可能为 0）
	tokenConsumed    int  // 令牌额度实际扣减量
	extraReserved    int  // 发送前补充预扣的额度（订阅退款时需要单独回滚）
	trusted          bool // 是否命中信任额度旁路
	fundingSettled   bool // funding.Settle 已成功，资金来源已提交
	settled          bool // Settle 全部完成（资金 + 令牌）
	refunded         bool // Refund 已调用
	// admissionSessionID + admissionAttempt identify every extra reserve
	// independently from request_id, so a replay's fresh pre-consume can never
	// reuse an earlier attempt's audit row.
	admissionSessionID     string
	admissionAttempt       int64
	refundSessionID        string
	refundOperationID      string
	refundStarted          bool
	requiresReconciliation bool
	mu                     sync.Mutex
}

// Settle 根据实际消耗额度进行结算。
// 资金来源和令牌额度分两步提交：若资金来源已提交但令牌调整失败，
// 会标记 fundingSettled 防止 Refund 对已提交的资金来源执行退款。
func (s *BillingSession) Settle(actualQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.requiresReconciliation {
		return ErrBillingSessionRequiresReconciliation
	}
	if s.refundStarted || s.refunded {
		return ErrBillingSessionRequiresReconciliation
	}
	if s.settled {
		return nil
	}
	delta := actualQuota - s.preConsumedQuota
	if delta == 0 {
		s.settled = true
		return nil
	}
	// 1) 调整资金来源（仅在尚未提交时执行，防止重复调用）
	if !s.fundingSettled {
		if err := s.funding.Settle(delta); err != nil {
			return err
		}
		s.fundingSettled = true
	}
	// 2) 调整令牌额度
	var tokenErr error
	if !s.relayInfo.IsPlayground {
		if delta > 0 {
			tokenErr = model.DecreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, delta)
		} else {
			tokenErr = model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, -delta)
		}
		if tokenErr != nil {
			// 资金来源已提交，令牌调整失败只能记录日志；标记 settled 防止 Refund 误退资金
			common.SysLog(fmt.Sprintf("error adjusting token quota after funding settled (userId=%d, tokenId=%d, delta=%d): %s",
				s.relayInfo.UserId, s.relayInfo.TokenId, delta, tokenErr.Error()))
		}
	}
	// 3) 更新 relayInfo 上的订阅 PostDelta（用于日志）
	if s.funding.Source() == BillingSourceSubscription {
		s.relayInfo.SubscriptionPostDelta += int64(delta)
	}
	s.settled = true
	return tokenErr
}

// Refund 退还所有预扣费，幂等安全，异步执行。每个外部退款动作开始前，
// BillingRefundOperation 都会把 ready 原子领取为 unknown；任何模糊结果
// 都会停止后续非幂等动作并要求人工对账。
func (s *BillingSession) Refund(c *gin.Context) {
	s.mu.Lock()
	if s.requiresReconciliation || s.settled || s.refunded || s.refundStarted || !s.needsRefundLocked() {
		s.mu.Unlock()
		return
	}

	if s.refundSessionID == "" {
		s.refundSessionID = uuid.NewString()
	}
	if s.refundOperationID == "" {
		s.refundOperationID = uuid.NewString()
	}
	requestID := strings.TrimSpace(s.relayInfo.RequestId)
	if requestID == "" {
		requestID = "billing-refund:" + s.refundSessionID
	}

	fundingQuota := 0
	fundingReferenceID := int64(0)
	subscriptionExtraQuota := 0
	switch funding := s.funding.(type) {
	case *WalletFunding:
		fundingQuota = funding.consumed
		fundingReferenceID = int64(funding.userId)
	case *SubscriptionFunding:
		if funding.preConsumed < 0 || funding.preConsumed > int64(common.MaxQuota) {
			s.mu.Unlock()
			common.SysLog(fmt.Sprintf("invalid subscription refund quota (requestId=%s, quota=%d)", requestID, funding.preConsumed))
			return
		}
		fundingQuota = int(funding.preConsumed)
		fundingReferenceID = int64(funding.subscriptionId)
		if s.extraReserved > 0 {
			subscriptionExtraQuota = s.extraReserved
		}
	default:
		s.mu.Unlock()
		common.SysLog("unsupported billing source for refund: " + s.funding.Source())
		return
	}
	tokenQuota := s.tokenConsumed
	if s.relayInfo.IsPlayground {
		tokenQuota = 0
	}

	operation, err := model.BeginBillingRefundOperation(model.BillingRefundOperationInput{
		OperationID:            s.refundOperationID,
		SessionID:              s.refundSessionID,
		RequestID:              requestID,
		UserID:                 s.relayInfo.UserId,
		TokenID:                s.relayInfo.TokenId,
		FundingSource:          s.funding.Source(),
		FundingReferenceID:     fundingReferenceID,
		FundingQuota:           fundingQuota,
		SubscriptionExtraQuota: subscriptionExtraQuota,
		TokenQuota:             tokenQuota,
	})
	if err != nil {
		// No external refund may start without a committed durable intent. A
		// definitely rolled-back intent is safe to retry only after persistence is
		// available again; a commit-unknown intent may already exist under the same
		// operation ID. Both outcomes therefore fail closed here.
		s.requiresReconciliation = true
		s.mu.Unlock()
		common.SysLog(fmt.Sprintf("error beginning billing refund operation (requestId=%s, operationId=%s): %s", requestID, s.refundOperationID, err.Error()))
		return
	}
	if operation.Status == model.BillingRefundStatusApplied {
		s.refunded = true
		s.mu.Unlock()
		return
	}
	if operation.Status != model.BillingRefundStatusPendingReconcile {
		s.requiresReconciliation = true
		s.mu.Unlock()
		common.SysLog(fmt.Sprintf("billing refund operation is not safe to replay (requestId=%s, operationId=%s, status=%s, pendingAction=%s)",
			requestID, s.refundOperationID, operation.Status, operation.PendingAction))
		return
	}

	s.refundStarted = true
	operationID := s.refundOperationID
	s.mu.Unlock()

	logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费（token_quota=%s, funding=%s）",
		s.relayInfo.UserId,
		logger.FormatQuota(tokenQuota),
		s.funding.Source(),
	))

	gopool.Go(func() {
		reconciled, reconcileErr := reconcilePendingBillingRefundOperation(operationID, false)
		if reconcileErr != nil || reconciled.Status != model.BillingRefundStatusApplied {
			if reconcileErr == nil {
				reconcileErr = ErrBillingRefundRequiresManualReconciliation
			}
			common.SysLog(fmt.Sprintf("billing refund requires reconciliation (operationId=%s, pendingAction=%s): %s", operationID, reconciled.PendingAction, reconcileErr.Error()))
			s.mu.Lock()
			s.requiresReconciliation = true
			s.mu.Unlock()
			return
		}
		s.mu.Lock()
		s.refunded = true
		s.refundStarted = false
		s.mu.Unlock()
	})
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

// FundingWasSettled reports whether the external funding source confirmed the
// final delta. A false value after an error is not proof that no side effect
// occurred; callers must still treat the settlement outcome as ambiguous.
func (s *BillingSession) FundingWasSettled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fundingSettled
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.requiresReconciliation || s.settled || s.refunded || s.refundStarted || s.fundingSettled {
		// fundingSettled 时资金来源已提交结算，不能再退预扣费
		return false
	}
	if s.tokenConsumed > 0 {
		return true
	}
	// 订阅可能在 tokenConsumed=0 时仍预扣了额度
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub.preConsumed > 0 {
		return true
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	return s.preConsumedQuota
}

func (s *BillingSession) Reserve(targetQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reserveLocked(targetQuota, false)
}

// ReserveForAdmission raises a reservation before an upstream task is sent.
// Unlike Reserve, its wallet branch must not create debt: it atomically checks
// and deducts the additional quota so a repriced auto-group retry fails closed
// before the provider can accept an asynchronous job.
func (s *BillingSession) ReserveForAdmission(targetQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reserveLocked(targetQuota, true)
}

func (s *BillingSession) reserveLocked(targetQuota int, strictWallet bool) error {
	if s.requiresReconciliation {
		return ErrBillingSessionRequiresReconciliation
	}
	if s.settled || s.refunded || s.refundStarted || s.trusted || targetQuota <= s.preConsumedQuota {
		return nil
	}

	delta := targetQuota - s.preConsumedQuota
	if delta <= 0 {
		return nil
	}

	if s.admissionSessionID == "" {
		s.admissionSessionID = uuid.NewString()
	}
	s.admissionAttempt++
	operationID := uuid.NewString()
	requestID := strings.TrimSpace(s.relayInfo.RequestId)
	if requestID == "" {
		requestID = "billing-session:" + s.admissionSessionID
	}
	fundingReferenceID := int64(0)
	switch funding := s.funding.(type) {
	case *WalletFunding:
		fundingReferenceID = int64(funding.userId)
	case *SubscriptionFunding:
		fundingReferenceID = int64(funding.subscriptionId)
	}
	mode := model.BillingAdmissionReserveModeStandard
	if _, isWallet := s.funding.(*WalletFunding); strictWallet && isWallet {
		mode = model.BillingAdmissionReserveModeStrictWallet
	}
	tokenQuota := delta
	if s.relayInfo.IsPlayground {
		tokenQuota = 0
	}
	_, err := model.BeginBillingAdmissionReserveOperation(model.BillingAdmissionReserveInput{
		OperationID:        operationID,
		SessionID:          s.admissionSessionID,
		RequestID:          requestID,
		Attempt:            s.admissionAttempt,
		UserID:             s.relayInfo.UserId,
		TokenID:            s.relayInfo.TokenId,
		FundingSource:      s.funding.Source(),
		FundingReferenceID: fundingReferenceID,
		FromQuota:          s.preConsumedQuota,
		TargetQuota:        targetQuota,
		TokenQuota:         tokenQuota,
		Mode:               mode,
	})
	if err != nil {
		// No top-up external action starts until Begin returns successfully.
		// Even when the intent write outcome is unknown, refunding the session's
		// already-existing pre-consume remains safe.
		return fmt.Errorf("begin billing admission reserve operation: %w", err)
	}
	runBillingBeforeActionClaimHook(operationID, model.BillingAdmissionReservePendingActionFundingReady)
	fundingClaim, err := model.ClaimNextBillingAdmissionReserveAction(operationID, model.BillingAdmissionReservePendingActionFundingReady)
	if err != nil || !fundingClaim.Claimed {
		s.requiresReconciliation = true
		if err == nil {
			err = model.ErrBillingAdmissionReserveInvalidTransition
		}
		return errors.Join(
			ErrBillingSessionRequiresReconciliation,
			fmt.Errorf("claim billing admission funding action: %w", err),
		)
	}

	var fundingErr error
	knownNotApplied := false
	if strictWallet {
		knownNotApplied, fundingErr = s.reserveFundingForAdmission(delta)
	} else {
		fundingErr = s.reserveFunding(delta)
	}
	if fundingErr != nil {
		if knownNotApplied {
			if cancelErr := model.CancelBillingAdmissionReserveOperation(operationID); cancelErr == nil {
				return fundingErr
			} else {
				// TryReserveUserQuota returned false,nil, which proves the top-up
				// moved no funding. A failed audit cancellation must not strand the
				// session's safe initial pre-consume.
				return errors.Join(
					fundingErr,
					fmt.Errorf("cancel known-not-applied billing admission reserve operation: %w", cancelErr),
				)
			}
		}
		s.requiresReconciliation = true
		return errors.Join(fundingErr, ErrBillingSessionRequiresReconciliation)
	}
	if err := model.ConfirmBillingAdmissionReserveFunding(operationID); err != nil {
		s.requiresReconciliation = true
		return errors.Join(
			ErrBillingSessionRequiresReconciliation,
			fmt.Errorf("confirm billing admission funding action: %w", err),
		)
	}
	if tokenQuota > 0 {
		runBillingBeforeActionClaimHook(operationID, model.BillingAdmissionReservePendingActionTokenReady)
		tokenClaim, claimErr := model.ClaimNextBillingAdmissionReserveAction(operationID, model.BillingAdmissionReservePendingActionTokenReady)
		if claimErr != nil || !tokenClaim.Claimed {
			s.requiresReconciliation = true
			if claimErr == nil {
				claimErr = model.ErrBillingAdmissionReserveInvalidTransition
			}
			return errors.Join(
				ErrBillingSessionRequiresReconciliation,
				fmt.Errorf("claim billing admission token action: %w", claimErr),
			)
		}
	}
	knownTokenNotApplied, tokenErr := s.reserveToken(tokenQuota)
	if tokenErr != nil {
		if knownTokenNotApplied {
			if rejectErr := model.RejectBillingAdmissionReserveToken(operationID); rejectErr != nil {
				s.requiresReconciliation = true
				return errors.Join(tokenErr, ErrBillingSessionRequiresReconciliation, fmt.Errorf("record rejected billing admission token action: %w", rejectErr))
			}
			if _, reconcileErr := ReconcilePendingBillingAdmissionReserveOperation(operationID); reconcileErr != nil {
				s.requiresReconciliation = true
				return errors.Join(tokenErr, ErrBillingSessionRequiresReconciliation, fmt.Errorf("compensate rejected billing admission token action: %w", reconcileErr))
			}
			return tokenErr
		}
		s.requiresReconciliation = true
		return errors.Join(tokenErr, ErrBillingSessionRequiresReconciliation)
	}
	if tokenQuota > 0 {
		if err := model.ConfirmBillingAdmissionReserveToken(operationID); err != nil {
			s.requiresReconciliation = true
			return errors.Join(
				ErrBillingSessionRequiresReconciliation,
				fmt.Errorf("confirm billing admission token action: %w", err),
			)
		}
	}
	if err := model.CommitBillingAdmissionReserveOperation(operationID); err != nil {
		s.requiresReconciliation = true
		return errors.Join(
			ErrBillingSessionRequiresReconciliation,
			fmt.Errorf("commit billing admission reserve operation: %w", err),
		)
	}

	s.preConsumedQuota += delta
	s.tokenConsumed += tokenQuota
	s.extraReserved += delta
	s.syncRelayInfo()
	return nil
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume executes the initial reservation through the same durable
// admission state machine used by later top-ups. Funding is claimed and
// confirmed before token quota, so a process loss always leaves either a
// replay-forbidden unknown state or confirmed evidence the worker can safely
// compensate.
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足, 信任且不需要预扣费 (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 需要预扣费 %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}

	if effectiveQuota == 0 {
		s.preConsumedQuota = 0
		s.syncRelayInfo()
		return nil
	}

	if s.admissionSessionID == "" {
		s.admissionSessionID = uuid.NewString()
	}
	operationID := uuid.NewString()
	requestID := strings.TrimSpace(s.relayInfo.RequestId)
	if requestID == "" {
		requestID = "billing-session:" + s.admissionSessionID
	}
	fundingReferenceID := int64(0)
	if wallet, ok := s.funding.(*WalletFunding); ok {
		fundingReferenceID = int64(wallet.userId)
	}
	tokenQuota := effectiveQuota
	if s.relayInfo.IsPlayground {
		tokenQuota = 0
	}
	_, err := model.BeginBillingAdmissionReserveOperation(model.BillingAdmissionReserveInput{
		OperationID:        operationID,
		SessionID:          s.admissionSessionID,
		RequestID:          requestID,
		Attempt:            0,
		UserID:             s.relayInfo.UserId,
		TokenID:            s.relayInfo.TokenId,
		FundingSource:      s.funding.Source(),
		FundingReferenceID: fundingReferenceID,
		FromQuota:          0,
		TargetQuota:        effectiveQuota,
		TokenQuota:         tokenQuota,
		Mode:               model.BillingAdmissionReserveModeInitial,
	})
	if err != nil {
		return types.NewError(fmt.Errorf("begin initial billing admission reserve operation: %w", err), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	runBillingBeforeActionClaimHook(operationID, model.BillingAdmissionReservePendingActionFundingReady)
	fundingClaim, err := model.ClaimNextBillingAdmissionReserveAction(operationID, model.BillingAdmissionReservePendingActionFundingReady)
	if err != nil || !fundingClaim.Claimed {
		if err == nil {
			err = model.ErrBillingAdmissionReserveInvalidTransition
		}
		return types.NewError(
			errors.Join(ErrBillingSessionRequiresReconciliation, fmt.Errorf("claim initial billing funding action: %w", err)),
			types.ErrorCodeUpdateDataError,
			types.ErrOptionWithSkipRetry(),
		)
	}

	knownFundingNotApplied := false
	var fundingErr error
	switch funding := s.funding.(type) {
	case *WalletFunding:
		reserved, reserveErr := model.ReserveUserQuotaForBilling(funding.userId, effectiveQuota, true)
		if reserveErr != nil {
			fundingErr = reserveErr
		} else if !reserved {
			knownFundingNotApplied = true
			fundingErr = ErrInsufficientWalletQuota
		} else {
			funding.consumed = effectiveQuota
		}
	case *SubscriptionFunding:
		fundingErr = funding.PreConsume(effectiveQuota)
		if fundingErr != nil {
			errMessage := fundingErr.Error()
			knownFundingNotApplied = strings.Contains(errMessage, "no active subscription") ||
				strings.Contains(errMessage, "subscription quota insufficient")
		}
	default:
		fundingErr = fmt.Errorf("unsupported funding source: %s", s.funding.Source())
	}
	if fundingErr != nil {
		if knownFundingNotApplied {
			if cancelErr := model.CancelBillingAdmissionReserveOperation(operationID); cancelErr != nil {
				return types.NewError(
					errors.Join(ErrBillingSessionRequiresReconciliation, fundingErr, fmt.Errorf("cancel rejected initial funding action: %w", cancelErr)),
					types.ErrorCodeUpdateDataError,
					types.ErrOptionWithSkipRetry(),
				)
			}
			if errors.Is(fundingErr, ErrInsufficientWalletQuota) {
				userQuota, quotaErr := model.GetUserQuota(s.relayInfo.UserId, false)
				if quotaErr != nil {
					userQuota = 0
				}
				return types.NewErrorWithStatusCode(
					fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)),
					types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
					types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			return types.NewErrorWithStatusCode(
				fmt.Errorf("订阅额度不足或未配置订阅: %s", fundingErr.Error()),
				types.ErrorCodeInsufficientUserQuota,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		return types.NewError(
			errors.Join(ErrBillingSessionRequiresReconciliation, fundingErr),
			types.ErrorCodeUpdateDataError,
			types.ErrOptionWithSkipRetry(),
		)
	}

	if funding, ok := s.funding.(*SubscriptionFunding); ok {
		err = model.ConfirmBillingAdmissionReserveFundingReference(operationID, int64(funding.subscriptionId))
	} else {
		err = model.ConfirmBillingAdmissionReserveFunding(operationID)
	}
	if err != nil {
		return types.NewError(
			errors.Join(ErrBillingSessionRequiresReconciliation, fmt.Errorf("confirm initial billing funding action: %w", err)),
			types.ErrorCodeUpdateDataError,
			types.ErrOptionWithSkipRetry(),
		)
	}

	if tokenQuota > 0 {
		runBillingBeforeActionClaimHook(operationID, model.BillingAdmissionReservePendingActionTokenReady)
		tokenClaim, claimErr := model.ClaimNextBillingAdmissionReserveAction(operationID, model.BillingAdmissionReservePendingActionTokenReady)
		if claimErr != nil || !tokenClaim.Claimed {
			if claimErr == nil {
				claimErr = model.ErrBillingAdmissionReserveInvalidTransition
			}
			return types.NewError(
				errors.Join(ErrBillingSessionRequiresReconciliation, fmt.Errorf("claim initial billing token action: %w", claimErr)),
				types.ErrorCodeUpdateDataError,
				types.ErrOptionWithSkipRetry(),
			)
		}
		reserved, reserveErr := model.ReserveTokenQuotaForBilling(
			s.relayInfo.TokenId,
			s.relayInfo.TokenKey,
			tokenQuota,
			s.relayInfo.TokenUnlimited,
		)
		if reserveErr != nil {
			return types.NewErrorWithStatusCode(
				errors.Join(ErrBillingSessionRequiresReconciliation, reserveErr),
				types.ErrorCodeUpdateDataError,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		if !reserved {
			remainQuota := 0
			if token, tokenErr := model.GetTokenById(s.relayInfo.TokenId); tokenErr == nil && token != nil {
				remainQuota = token.RemainQuota
			}
			tokenErr := types.NewErrorWithStatusCode(
				fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(remainQuota), logger.FormatQuota(tokenQuota)),
				types.ErrorCodePreConsumeTokenQuotaFailed,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
			if rejectErr := model.RejectBillingAdmissionReserveToken(operationID); rejectErr != nil {
				return types.NewError(
					errors.Join(ErrBillingSessionRequiresReconciliation, tokenErr.Err, fmt.Errorf("record rejected initial token action: %w", rejectErr)),
					types.ErrorCodeUpdateDataError,
					types.ErrOptionWithSkipRetry(),
				)
			}
			if _, reconcileErr := ReconcilePendingBillingAdmissionReserveOperation(operationID); reconcileErr != nil {
				return types.NewError(
					errors.Join(ErrBillingSessionRequiresReconciliation, tokenErr.Err, fmt.Errorf("compensate rejected initial token action: %w", reconcileErr)),
					types.ErrorCodeUpdateDataError,
					types.ErrOptionWithSkipRetry(),
				)
			}
			return tokenErr
		}
		if err := model.ConfirmBillingAdmissionReserveToken(operationID); err != nil {
			return types.NewError(
				errors.Join(ErrBillingSessionRequiresReconciliation, fmt.Errorf("confirm initial billing token action: %w", err)),
				types.ErrorCodeUpdateDataError,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}
	if err := model.CommitBillingAdmissionReserveOperation(operationID); err != nil {
		return types.NewError(
			errors.Join(ErrBillingSessionRequiresReconciliation, fmt.Errorf("commit initial billing admission reserve operation: %w", err)),
			types.ErrorCodeUpdateDataError,
			types.ErrOptionWithSkipRetry(),
		)
	}

	s.preConsumedQuota = effectiveQuota
	// Preserve the logical cursor for playground sessions even though token quota
	// is intentionally not debited there.
	s.tokenConsumed = effectiveQuota

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

func (s *BillingSession) reserveFunding(delta int) error {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		// 与结算补扣（SettleBilling 正差额 → WalletFunding.Settle）语义一致：
		// 全额无条件扣减，余额不足的部分记为欠费（余额可为负），不中断请求，
		// 保证日志记录的预扣额度与用户余额的实际变动始终对账一致。
		reserved, err := model.ReserveUserQuotaForBilling(funding.userId, delta, false)
		if err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		if !reserved {
			return types.NewError(errors.New("billing wallet quota reserve was not applied"), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		funding.consumed += delta
		return nil
	case *SubscriptionFunding:
		if err := model.PostConsumeUserSubscriptionDelta(funding.subscriptionId, int64(delta)); err != nil {
			return types.NewErrorWithStatusCode(
				fmt.Errorf("订阅额度不足或未配置订阅: %s", err.Error()),
				types.ErrorCodeInsufficientUserQuota,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		return nil
	default:
		return types.NewError(fmt.Errorf("unsupported funding source: %s", s.funding.Source()), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
}

func (s *BillingSession) reserveFundingForAdmission(delta int) (bool, error) {
	if funding, ok := s.funding.(*WalletFunding); ok {
		reserved, err := model.ReserveUserQuotaForBilling(funding.userId, delta, true)
		if err != nil {
			return false, types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		if !reserved {
			return true, types.NewErrorWithStatusCode(
				fmt.Errorf("预扣费额度失败, 需要补充预扣费额度: %s", logger.FormatQuota(delta)),
				types.ErrorCodeInsufficientUserQuota,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		funding.consumed += delta
		return false, nil
	}
	// Subscription deltas are already constrained transactionally by
	// PostConsumeUserSubscriptionDelta, so the regular reserve path is strict.
	return false, s.reserveFunding(delta)
}

func (s *BillingSession) reserveToken(delta int) (bool, error) {
	if delta <= 0 || s.relayInfo.IsPlayground {
		return false, nil
	}
	reserved, err := model.ReserveTokenQuotaForBilling(s.relayInfo.TokenId, s.relayInfo.TokenKey, delta, s.relayInfo.TokenUnlimited)
	if err != nil {
		return false, types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if !reserved {
		return true, types.NewErrorWithStatusCode(
			fmt.Errorf("token quota is not enough, need quota: %s", logger.FormatQuota(delta)),
			types.ErrorCodePreConsumeTokenQuotaFailed,
			http.StatusForbidden,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	return false, nil
}

// shouldTrust 统一信任额度检查，适用于钱包和订阅。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := common.GetTrustQuota()
	if trustQuota <= 0 {
		return false
	}

	// 检查令牌是否充足
	tokenTrusted := s.relayInfo.TokenUnlimited
	if !tokenTrusted {
		tokenQuota := c.GetInt("token_quota")
		tokenTrusted = tokenQuota > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	switch s.funding.Source() {
	case BillingSourceWallet:
		return s.relayInfo.UserQuota > trustQuota
	case BillingSourceSubscription:
		// 订阅不能启用信任旁路。原因：
		// 1. PreConsumeUserSubscription 要求 amount>0 来创建预扣记录并锁定订阅
		// 2. SubscriptionFunding.PreConsume 忽略参数，始终用 s.amount 预扣
		// 3. 若信任旁路将 effectiveQuota 设为 0，会导致 preConsumedQuota 与实际订阅预扣不一致
		return false
	default:
		return false
	}
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = sub.subscriptionId
		info.SubscriptionPreConsumed = sub.preConsumed + int64(s.extraReserved)
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = sub.AmountTotal
		info.SubscriptionAmountUsedAfterPreConsume = sub.AmountUsedAfter + int64(s.extraReserved)
		info.SubscriptionPlanId = sub.PlanId
		info.SubscriptionPlanTitle = sub.PlanTitle
	} else {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
	}
}

// ---------------------------------------------------------------------------
// NewBillingSession 工厂 — 根据计费偏好创建会话并处理回退
// ---------------------------------------------------------------------------

// NewBillingSession 根据用户计费偏好创建 BillingSession，处理 subscription_first / wallet_first 的回退。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	pref := common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference)

	// 钱包路径需要先检查用户额度
	tryWallet := func() (*BillingSession, *types.NewAPIError) {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if userQuota <= 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if userQuota-preConsumedQuota < 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota(userQuota), logger.FormatQuota(preConsumedQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.UserQuota = userQuota

		session := &BillingSession{
			relayInfo:          relayInfo,
			funding:            &WalletFunding{userId: relayInfo.UserId},
			admissionSessionID: uuid.NewString(),
		}
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	trySubscription := func() (*BillingSession, *types.NewAPIError) {
		subConsume := int64(preConsumedQuota)
		if subConsume <= 0 {
			subConsume = 1
		}
		session := &BillingSession{
			relayInfo:          relayInfo,
			admissionSessionID: uuid.NewString(),
			funding: &SubscriptionFunding{
				requestId: relayInfo.RequestId,
				userId:    relayInfo.UserId,
				modelName: relayInfo.OriginModelName,
				amount:    subConsume,
			},
		}
		// 必须传 subConsume 而非 preConsumedQuota，保证 SubscriptionFunding.amount、
		// preConsume 参数和 FinalPreConsumedQuota 三者一致，避免订阅多扣费。
		if apiErr := session.preConsume(c, int(subConsume)); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	switch pref {
	case "subscription_only":
		return trySubscription()
	case "wallet_only":
		return tryWallet()
	case "wallet_first":
		session, err := tryWallet()
		if err != nil {
			if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return trySubscription()
			}
			return nil, err
		}
		return session, nil
	case "subscription_first":
		fallthrough
	default:
		hasSub, subCheckErr := model.HasActiveUserSubscription(relayInfo.UserId)
		if subCheckErr != nil {
			return nil, types.NewError(subCheckErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if !hasSub {
			return tryWallet()
		}
		session, apiErr := trySubscription()
		if apiErr != nil {
			if apiErr.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				// 仅当用户的活跃订阅允许钱包回退时才回退到钱包，否则返回订阅额度不足错误
				allowOverflow, overflowErr := model.UserActiveSubscriptionsAllowWalletOverflow(relayInfo.UserId)
				if overflowErr != nil {
					return nil, types.NewError(overflowErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
				}
				if allowOverflow {
					return tryWallet()
				}
				return nil, apiErr
			}
			return nil, apiErr
		}
		return session, nil
	}
}
