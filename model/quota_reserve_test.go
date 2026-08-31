package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createReserveTestUser(t *testing.T, quota int) User {
	t.Helper()
	user := User{
		Username:    "reserve-user-" + common.GetRandomString(6),
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       quota,
		AffCode:     "reserve-aff-" + common.GetRandomString(8),
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func createReserveTestToken(t *testing.T, remainQuota int) Token {
	t.Helper()
	token := Token{
		UserId:      1,
		Key:         "reserve-token-" + common.GetRandomString(8),
		Name:        "reserve-test",
		Status:      common.TokenStatusEnabled,
		ExpiredTime: -1,
		RemainQuota: remainQuota,
	}
	require.NoError(t, token.Insert())
	return token
}

func getUserQuotaFromDB(t *testing.T, id int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").First(&user, id).Error)
	return user.Quota
}

func getUserQuotaVersionFromDB(t *testing.T, id int) int64 {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota_version").First(&user, id).Error)
	return user.QuotaVersion
}

func getTokenFromDB(t *testing.T, id int) Token {
	t.Helper()
	var token Token
	require.NoError(t, DB.First(&token, id).Error)
	return token
}

func resetBatchUpdateTestState(t *testing.T) {
	t.Helper()
	oldBatchEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchEnabled
		for i := 0; i < BatchUpdateTypeCount; i++ {
			batchUpdateLocks[i].Lock()
			batchUpdateStores[i] = make(map[int]int)
			batchUpdateLocks[i].Unlock()
		}
	})
}

func setQuotaCacheRaceHooksForTest(t *testing.T, userAfterVersionCheck, tokenAfterVersionCheck, userAfterDBMutation, tokenAfterDBMutation func()) {
	t.Helper()
	quotaCacheRaceHooks.Lock()
	quotaCacheRaceHooks.userAfterVersionCheck = userAfterVersionCheck
	quotaCacheRaceHooks.tokenAfterVersionCheck = tokenAfterVersionCheck
	quotaCacheRaceHooks.userAfterDBMutation = userAfterDBMutation
	quotaCacheRaceHooks.tokenAfterDBMutation = tokenAfterDBMutation
	quotaCacheRaceHooks.Unlock()
	t.Cleanup(func() {
		quotaCacheRaceHooks.Lock()
		quotaCacheRaceHooks.userAfterVersionCheck = nil
		quotaCacheRaceHooks.tokenAfterVersionCheck = nil
		quotaCacheRaceHooks.userAfterDBMutation = nil
		quotaCacheRaceHooks.tokenAfterDBMutation = nil
		quotaCacheRaceHooks.Unlock()
	})
}

func setQuotaCacheBeforeDBMutationHooksForTest(t *testing.T, userBeforeDBMutation, tokenBeforeDBMutation func()) {
	t.Helper()
	quotaCacheRaceHooks.Lock()
	quotaCacheRaceHooks.userBeforeDBMutation = userBeforeDBMutation
	quotaCacheRaceHooks.tokenBeforeDBMutation = tokenBeforeDBMutation
	quotaCacheRaceHooks.Unlock()
	t.Cleanup(func() {
		quotaCacheRaceHooks.Lock()
		quotaCacheRaceHooks.userBeforeDBMutation = nil
		quotaCacheRaceHooks.tokenBeforeDBMutation = nil
		quotaCacheRaceHooks.Unlock()
	})
}

func waitForQuotaRaceSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for quota-cache race signal", description)
	}
}

func TestTryReserveQuotaWithoutRedis(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)

	user := createReserveTestUser(t, 100)
	reserved, err := TryReserveUserQuota(user.Id, 60)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 40, getUserQuotaFromDB(t, user.Id))

	reserved, err = TryReserveUserQuota(user.Id, 41)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 40, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 80)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 25, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 55, reloaded.RemainQuota)
	assert.Equal(t, 25, reloaded.UsedQuota)

	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 56, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 55, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestRedisReservePersistsBalanceBeforeReturningWhenBatchingIsEnabled(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id), "balance deltas must bypass the batch queue")

	reserved, err = TryReserveUserQuota(user.Id, 3)
	require.NoError(t, err)
	assert.False(t, reserved, "stale DB balance must not authorize a second spend")
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, cachedUser.Quota)

	token := createReserveTestToken(t, 9)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 3, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Equal(t, 2, getTokenFromDB(t, token.Id).RemainQuota)

	batchUpdate()
	assert.Equal(t, 2, getUserQuotaFromDB(t, user.Id))
	reloadedToken := getTokenFromDB(t, token.Id)
	assert.Equal(t, 2, reloadedToken.RemainQuota)
	assert.Equal(t, 7, reloadedToken.UsedQuota)
}

func TestReservePersistsAgainstDatabaseWhenRedisIsUnavailableWithoutDoubleSpend(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 20)
	require.NoError(t, populateUserCache(user))
	token := createReserveTestToken(t, 20)
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	server.Close()

	reserved, err := TryReserveUserQuota(user.Id, 5)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 15, getUserQuotaFromDB(t, user.Id))
	assert.EqualValues(t, 1, getUserQuotaVersionFromDB(t, user.Id))

	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 5, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Equal(t, 15, getTokenFromDB(t, token.Id).RemainQuota)
	assert.EqualValues(t, 1, getTokenFromDB(t, token.Id).QuotaVersion)

	require.NoError(t, server.Restart())
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 15, cachedUser.Quota)
	cachedToken, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 15, cachedToken.RemainQuota)

	reserved, err = TryReserveUserQuota(user.Id, 15)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.Zero(t, getUserQuotaFromDB(t, user.Id))

	reserved, err = TryReserveUserQuota(user.Id, 6)
	require.NoError(t, err)
	assert.False(t, reserved)

	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 15, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 6, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.Zero(t, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestSynchronousReserveFencesCacheBeforeAuthoritativeDatabaseWrite(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, DB.Delete(&user).Error)

	reserved, err := TryReserveUserQuota(user.Id, 6)
	assert.False(t, reserved)
	assert.NoError(t, err)
	_, cacheErr := cacheGetUserBase(user.Id)
	assert.Error(t, cacheErr, "the stale user hash must be fenced before the authoritative DB reserve")

	token := createReserveTestToken(t, 12)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	require.NoError(t, DB.Delete(&token).Error)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	assert.False(t, reserved)
	assert.NoError(t, err)
	_, cacheErr = cacheGetTokenByKey(token.Key)
	assert.Error(t, cacheErr, "the stale token hash must be fenced before the authoritative DB reserve")
}

func TestLedgerBackedQuotaMutationsPersistSynchronouslyWhenBatchingIsEnabled(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 100)
	assert.Zero(t, getUserQuotaVersionFromDB(t, user.Id), "legacy zero is a valid initial quota version")
	reserved, err := ReserveUserQuotaForBilling(user.Id, 30, true)
	require.NoError(t, err)
	require.True(t, reserved)
	assert.Equal(t, 70, getUserQuotaFromDB(t, user.Id))
	assert.EqualValues(t, 1, getUserQuotaVersionFromDB(t, user.Id))
	require.NoError(t, RefundUserQuotaForBilling(user.Id, 30))
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
	assert.EqualValues(t, 2, getUserQuotaVersionFromDB(t, user.Id))

	token := createReserveTestToken(t, 80)
	reserved, err = ReserveTokenQuotaForBilling(token.Id, token.Key, 25, false)
	require.NoError(t, err)
	require.True(t, reserved)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 55, reloaded.RemainQuota)
	assert.Equal(t, 25, reloaded.UsedQuota)
	assert.EqualValues(t, 1, reloaded.QuotaVersion)
	require.NoError(t, RefundTokenQuotaForBilling(token.Id, token.Key, 25))
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 80, reloaded.RemainQuota)
	assert.Zero(t, reloaded.UsedQuota)
	assert.EqualValues(t, 2, reloaded.QuotaVersion)

	batchUpdate()
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id), "ledger-backed mutations must never enter the batch queue")
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 80, reloaded.RemainQuota)
	assert.Zero(t, reloaded.UsedQuota)
}

func TestLegacyBalanceMutationsCannotFlushAfterLedgerBackedRefund(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 100)
	require.NoError(t, DecreaseUserQuota(user.Id, 30, false))
	assert.Equal(t, 70, getUserQuotaFromDB(t, user.Id), "legacy user debits must persist before returning")
	require.NoError(t, IncreaseUserQuota(user.Id, 5, false))
	assert.Equal(t, 75, getUserQuotaFromDB(t, user.Id), "legacy user credits must persist before returning")
	require.NoError(t, RefundUserQuotaForBilling(user.Id, 5))
	assert.Equal(t, 80, getUserQuotaFromDB(t, user.Id))
	reserved, err := ReserveUserQuotaForBilling(user.Id, 81, true)
	require.NoError(t, err)
	assert.False(t, reserved)

	token := createReserveTestToken(t, 100)
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 30))
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 70, reloaded.RemainQuota, "legacy token debits must persist before returning")
	assert.Equal(t, 30, reloaded.UsedQuota)
	require.NoError(t, IncreaseTokenQuota(token.Id, token.Key, 5))
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 75, reloaded.RemainQuota, "legacy token credits must persist before returning")
	assert.Equal(t, 25, reloaded.UsedQuota)
	require.NoError(t, RefundTokenQuotaForBilling(token.Id, token.Key, 5))
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 80, reloaded.RemainQuota)
	assert.Equal(t, 20, reloaded.UsedQuota)
	reserved, err = ReserveTokenQuotaForBilling(token.Id, token.Key, 81, false)
	require.NoError(t, err)
	assert.False(t, reserved)

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	assert.Empty(t, batchUpdateStores[BatchUpdateTypeUserQuota], "user balances must never enter the process-local batch queue")
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	assert.Empty(t, batchUpdateStores[BatchUpdateTypeTokenQuota], "token balances must never enter the process-local batch queue")
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()

	batchUpdate()
	assert.Equal(t, 80, getUserQuotaFromDB(t, user.Id), "a later flush must not replay a pre-refund user delta")
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 80, reloaded.RemainQuota, "a later flush must not replay a pre-refund token delta")
	assert.Equal(t, 20, reloaded.UsedQuota)
}

func TestBatchUpdateDiscardsLegacyBalanceStoresWithoutMutation(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 80)
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, 25)
	addNewRecord(BatchUpdateTypeTokenQuota, token.Id, -20)

	batchUpdate()
	batchUpdate()

	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
	assert.Zero(t, getUserQuotaVersionFromDB(t, user.Id))
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 80, reloaded.RemainQuota)
	assert.Zero(t, reloaded.UsedQuota)
	assert.Zero(t, reloaded.QuotaVersion)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	assert.Empty(t, batchUpdateStores[BatchUpdateTypeUserQuota])
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	assert.Empty(t, batchUpdateStores[BatchUpdateTypeTokenQuota])
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
}

func TestTransactionalQuotaMutationsAdvanceVersionAndLeaveCacheCold(t *testing.T) {
	t.Run("affiliate quota transfer", func(t *testing.T) {
		truncateTables(t)
		useUserCacheMiniRedis(t)
		previousQuotaPerUnit := common.QuotaPerUnit
		common.QuotaPerUnit = 1
		t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

		user := createReserveTestUser(t, 100)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("aff_quota", 75).Error)
		user.AffQuota = 75
		require.NoError(t, populateUserCache(user))

		require.NoError(t, user.TransferAffQuotaToQuota(25))

		reloaded := User{}
		require.NoError(t, DB.First(&reloaded, user.Id).Error)
		assert.Equal(t, 125, reloaded.Quota)
		assert.Equal(t, 50, reloaded.AffQuota)
		assert.EqualValues(t, 1, reloaded.QuotaVersion)
		_, err := cacheGetUserBase(user.Id)
		assert.Error(t, err, "affiliate transfer must leave Redis cold behind the quota fence")
	})

	t.Run("checkin credit", func(t *testing.T) {
		truncateTables(t)
		useUserCacheMiniRedis(t)
		require.NoError(t, DB.AutoMigrate(&Checkin{}))
		require.NoError(t, DB.Exec("DELETE FROM checkins").Error)
		t.Cleanup(func() { require.NoError(t, DB.Exec("DELETE FROM checkins").Error) })

		user := createReserveTestUser(t, 100)
		require.NoError(t, populateUserCache(user))
		checkin := &Checkin{
			UserId:       user.Id,
			CheckinDate:  "2099-01-01",
			QuotaAwarded: 25,
			CreatedAt:    time.Now().Unix(),
		}
		_, err := userCheckinWithTransaction(checkin, user.Id, 25)
		require.NoError(t, err)

		reloaded := User{}
		require.NoError(t, DB.First(&reloaded, user.Id).Error)
		assert.Equal(t, 125, reloaded.Quota)
		assert.EqualValues(t, 1, reloaded.QuotaVersion)
		_, err = cacheGetUserBase(user.Id)
		assert.Error(t, err, "transactional credit must leave Redis cold behind the quota fence")
	})

	t.Run("subscription balance debit", func(t *testing.T) {
		truncateTables(t)
		useUserCacheMiniRedis(t)

		previousQuotaPerUnit := common.QuotaPerUnit
		common.QuotaPerUnit = 100
		t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

		user := createReserveTestUser(t, 500)
		require.NoError(t, populateUserCache(user))
		allowBalancePay := true
		plan := SubscriptionPlan{
			Title:            "quota-version-balance-plan-" + common.GetRandomString(6),
			PriceAmount:      2,
			Currency:         "USD",
			DurationUnit:     SubscriptionDurationMonth,
			DurationValue:    1,
			Enabled:          true,
			AllowBalancePay:  &allowBalancePay,
			TotalAmount:      1_000,
			QuotaResetPeriod: SubscriptionResetNever,
		}
		require.NoError(t, DB.Create(&plan).Error)

		require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))

		reloaded := User{}
		require.NoError(t, DB.First(&reloaded, user.Id).Error)
		assert.Equal(t, 300, reloaded.Quota)
		assert.EqualValues(t, 1, reloaded.QuotaVersion)
		_, err := cacheGetUserBase(user.Id)
		assert.Error(t, err, "transactional debit must leave Redis cold behind the quota fence")
	})
}

func TestLedgerBackedQuotaMutationsKeepRedisAndDatabaseInSync(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 100)
	require.NoError(t, populateUserCache(user))
	reserved, err := ReserveUserQuotaForBilling(user.Id, 35, true)
	require.NoError(t, err)
	require.True(t, reserved)
	assert.Equal(t, 65, getUserQuotaFromDB(t, user.Id))
	_, err = cacheGetUserBase(user.Id)
	assert.Error(t, err, "a synchronous ledger debit keeps the cache cold behind its mutation fence")
	require.NoError(t, RefundUserQuotaForBilling(user.Id, 35))
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
	_, err = cacheGetUserBase(user.Id)
	assert.Error(t, err, "refund keeps the user cache cold behind a mutation fence")
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 100, cachedUser.Quota)
	_, err = cacheGetUserBase(user.Id)
	assert.Error(t, err, "a DB-only read must not populate quota while the mutation fence is active")

	token := createReserveTestToken(t, 90)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	reserved, err = ReserveTokenQuotaForBilling(token.Id, token.Key, 40, false)
	require.NoError(t, err)
	require.True(t, reserved)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 50, reloaded.RemainQuota)
	assert.Equal(t, 40, reloaded.UsedQuota)
	_, err = cacheGetTokenByKey(token.Key)
	assert.Error(t, err, "a synchronous token debit keeps the cache cold behind its mutation fence")
	require.NoError(t, RefundTokenQuotaForBilling(token.Id, token.Key, 40))
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 90, reloaded.RemainQuota)
	assert.Zero(t, reloaded.UsedQuota)
	_, err = cacheGetTokenByKey(token.Key)
	assert.Error(t, err, "refund keeps the token cache cold behind a mutation fence")
	cachedToken, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 90, cachedToken.RemainQuota)
	assert.Zero(t, cachedToken.UsedQuota)
	_, err = cacheGetTokenByKey(token.Key)
	assert.Error(t, err, "a DB-only read must not populate token quota while the mutation fence is active")

	batchUpdate()
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, 90, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestLedgerBackedRefundPersistsAgainstDatabaseWhenCacheFenceCannotBePublished(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 100)
	require.NoError(t, populateUserCache(user))
	server.Close()

	err := RefundUserQuotaForBilling(user.Id, 20)

	require.NoError(t, err)
	assert.Equal(t, 120, getUserQuotaFromDB(t, user.Id))
	assert.EqualValues(t, 1, getUserQuotaVersionFromDB(t, user.Id))
	require.NoError(t, server.Restart())
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 120, cachedUser.Quota)

	token := createReserveTestToken(t, 80)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("used_quota", 20).Error)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	server.Close()

	err = RefundTokenQuotaForBilling(token.Id, token.Key, 10)

	require.NoError(t, err)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 90, reloaded.RemainQuota)
	assert.Equal(t, 10, reloaded.UsedQuota)
	assert.EqualValues(t, 1, reloaded.QuotaVersion)
	require.NoError(t, server.Restart())
	cachedToken, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 90, cachedToken.RemainQuota)
}

func TestLedgerBackedRefundFencesDelayedStaleCacheFill(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 100)
	staleUser := user.ToBaseUser()
	require.NoError(t, RefundUserQuotaForBilling(user.Id, 20))
	assert.Equal(t, 120, getUserQuotaFromDB(t, user.Id))

	staleWriteErr := writeUserCache(staleUser, true)
	assert.Error(t, staleWriteErr, "a reader holding a pre-refund DB snapshot must not publish it after the refund")
	_, cacheErr := cacheGetUserBase(user.Id)
	assert.Error(t, cacheErr)
	freshUser, err := GetUserCache(user.Id)
	require.NoError(t, err, "readers inside the quota fence must use the database")
	assert.Equal(t, 120, freshUser.Quota)
	_, cacheErr = cacheGetUserBase(user.Id)
	assert.Error(t, cacheErr, "the fresh DB-only read must leave the cache cold until the fence expires")
	reserved, err := ReserveUserQuotaForBilling(user.Id, 90, true)
	require.NoError(t, err, "a new synchronous debit must remain available while the refund fence is active")
	require.True(t, reserved)
	assert.Equal(t, 30, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 80)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("used_quota", 20).Error)
	staleToken := getTokenFromDB(t, token.Id)
	require.NoError(t, RefundTokenQuotaForBilling(token.Id, token.Key, 10))
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 90, reloaded.RemainQuota)
	assert.Equal(t, 10, reloaded.UsedQuota)

	initResult, err := cacheInitToken(staleToken)
	assert.ErrorIs(t, err, ErrTokenQuotaCacheStale)
	assert.Zero(t, initResult, "the token mutation fence must reject a pre-refund DB snapshot")
	freshToken, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 90, freshToken.RemainQuota)
	assert.Equal(t, 10, freshToken.UsedQuota)
	_, cacheErr = cacheGetTokenByKey(token.Key)
	assert.Error(t, cacheErr, "token reads must remain DB-only until the fence expires")
	reserved, err = ReserveTokenQuotaForBilling(token.Id, token.Key, 70, false)
	require.NoError(t, err, "wallet-first subscription fallback must be able to reserve behind the rollback fence")
	require.True(t, reserved)
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 20, reloaded.RemainQuota)
	assert.Equal(t, 80, reloaded.UsedQuota)

	server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
	staleWriteErr = writeUserCache(staleUser, true)
	assert.ErrorIs(t, staleWriteErr, ErrUserQuotaCacheStale, "an arbitrarily delayed user snapshot must remain stale after the short fence expires")
	_, cacheErr = cacheGetUserBase(user.Id)
	assert.Error(t, cacheErr)
	reserved, err = ReserveUserQuotaForBilling(user.Id, staleUser.Quota, true)
	require.NoError(t, err)
	assert.False(t, reserved, "strict reserve must use the authoritative database guard, never the stale cache balance")
	assert.Equal(t, 30, getUserQuotaFromDB(t, user.Id), "strict reserve must never drive the database negative")

	initResult, err = cacheInitToken(staleToken)
	assert.ErrorIs(t, err, ErrTokenQuotaCacheStale, "an arbitrarily delayed token snapshot must remain stale after the short fence expires")
	assert.Zero(t, initResult)
	_, cacheErr = cacheGetTokenByKey(token.Key)
	assert.Error(t, cacheErr)
	reserved, err = ReserveTokenQuotaForBilling(token.Id, token.Key, staleToken.RemainQuota, false)
	require.NoError(t, err)
	assert.False(t, reserved, "strict token reserve must use the authoritative database guard")
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 20, reloaded.RemainQuota)
	assert.Equal(t, 80, reloaded.UsedQuota)

	server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
	freshUser, err = GetUserCache(user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 2, freshUser.QuotaVersion)
	cachedUser, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 2, cachedUser.QuotaVersion, "the user cache must carry the durable quota generation")
	freshToken, err = GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.EqualValues(t, 2, freshToken.QuotaVersion)
	cachedToken, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.EqualValues(t, 2, cachedToken.QuotaVersion, "the token cache must carry the durable quota generation")
}

func TestTokenCacheInitPreservesLiveQuotaAndFenceBlocksStaleSnapshot(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	token := createReserveTestToken(t, 100)
	loaded, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	stale := *loaded
	require.NoError(t, common.RDB.HDel(t.Context(), getTokenCacheKey(token.Key), "QuotaVersion").Err())
	_, err = cacheGetTokenByKey(token.Key)
	assert.Error(t, err, "a pre-upgrade token hash without quota version must never be trusted")
	code, err := cacheInitToken(stale)
	require.NoError(t, err)
	assert.Equal(t, 1, code, "a versionless token hash must be replaced from the verified DB snapshot")

	result, err := cacheApplyTokenQuotaDelta(token.Id, token.Key, -70)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)

	// 已存在的哈希只刷新 TTL：数据库快照不得覆盖已被原子预扣的余额。
	code, err = cacheInitToken(stale)
	require.NoError(t, err)
	assert.Equal(t, 2, code)
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 30, cached.RemainQuota)

	// 变更期间：fence 删除缓存并拦截并发读者手中的过期快照。
	require.NoError(t, invalidateTokenCacheForMutation(token.Key))
	code, err = cacheInitToken(stale)
	require.NoError(t, err)
	assert.Zero(t, code, "the pre-mutation snapshot must not be published while fenced")
	_, err = cacheGetTokenByKey(token.Key)
	assert.Error(t, err)

	// fence 过期后可重新从数据库水合。
	server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
	fresh, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 100, fresh.RemainQuota)
	cached, err = cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 100, cached.RemainQuota)
}

func TestFreshQuotaVersionRepairsAnOlderLiveCacheHash(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 100)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         45,
		"quota_version": 1,
	}).Error)
	var freshUser User
	require.NoError(t, DB.First(&freshUser, user.Id).Error)
	require.NoError(t, populateUserCache(freshUser))
	cachedUser, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 45, cachedUser.Quota)
	assert.EqualValues(t, 1, cachedUser.QuotaVersion)

	token := createReserveTestToken(t, 80)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
		"remain_quota":  30,
		"used_quota":    50,
		"quota_version": 1,
	}).Error)
	freshToken := getTokenFromDB(t, token.Id)
	result, err := cacheInitToken(freshToken)
	require.NoError(t, err)
	assert.Equal(t, 1, result)
	cachedToken, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 30, cachedToken.RemainQuota)
	assert.Equal(t, 50, cachedToken.UsedQuota)
	assert.EqualValues(t, 1, cachedToken.QuotaVersion)
}

func TestSetUserQuotaPersistsWhenRedisIsUnavailableAndAdvancesQuotaVersion(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 100)
	require.NoError(t, populateUserCache(user))
	server.Close()

	err := SetUserQuota(user.Id, 42)
	require.NoError(t, err)
	assert.Equal(t, 42, getUserQuotaFromDB(t, user.Id))
	assert.EqualValues(t, 1, getUserQuotaVersionFromDB(t, user.Id))

	require.NoError(t, server.Restart())
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 42, cached.Quota)
	assert.EqualValues(t, 1, cached.QuotaVersion)
}

func TestQuotaCachePublisherCannotOutliveFenceAndRepopulateStaleBalance(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		truncateTables(t)
		resetBatchUpdateTestState(t)
		server := useUserCacheMiniRedis(t)

		user := createReserveTestUser(t, 100)
		staleSnapshot := user.ToBaseUser()
		publisherChecked := make(chan struct{})
		releasePublisher := make(chan struct{})
		mutationPersisted := make(chan struct{})
		releaseMutation := make(chan struct{})
		setQuotaCacheRaceHooksForTest(t,
			func() {
				close(publisherChecked)
				<-releasePublisher
			},
			nil,
			func() {
				close(mutationPersisted)
				<-releaseMutation
			},
			nil,
		)

		publisherResult := make(chan error, 1)
		go func() { publisherResult <- writeUserCache(staleSnapshot, true) }()
		waitForQuotaRaceSignal(t, publisherChecked, "user publisher DB-version precheck")

		type reserveResult struct {
			reserved bool
			err      error
		}
		mutationResult := make(chan reserveResult, 1)
		go func() {
			reserved, err := ReserveUserQuotaForBilling(user.Id, 25, true)
			mutationResult <- reserveResult{reserved: reserved, err: err}
		}()
		waitForQuotaRaceSignal(t, mutationPersisted, "user quota DB mutation")

		server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
		close(releasePublisher)
		assert.ErrorIs(t, <-publisherResult, ErrUserQuotaCacheStale)
		_, err := cacheGetUserBase(user.Id)
		assert.Error(t, err, "the stale user snapshot must be removed by its post-publish version check")

		close(releaseMutation)
		result := <-mutationResult
		require.NoError(t, result.err)
		assert.True(t, result.reserved)
		assert.Equal(t, 75, getUserQuotaFromDB(t, user.Id))
		assert.EqualValues(t, 1, getUserQuotaVersionFromDB(t, user.Id))
		_, err = cacheGetUserBase(user.Id)
		assert.Error(t, err, "the successful mutation must publish a second fence after commit")
	})

	t.Run("token", func(t *testing.T) {
		truncateTables(t)
		resetBatchUpdateTestState(t)
		server := useUserCacheMiniRedis(t)

		token := createReserveTestToken(t, 100)
		staleSnapshot := getTokenFromDB(t, token.Id)
		publisherChecked := make(chan struct{})
		releasePublisher := make(chan struct{})
		mutationPersisted := make(chan struct{})
		releaseMutation := make(chan struct{})
		setQuotaCacheRaceHooksForTest(t,
			nil,
			func() {
				close(publisherChecked)
				<-releasePublisher
			},
			nil,
			func() {
				close(mutationPersisted)
				<-releaseMutation
			},
		)

		publisherResult := make(chan error, 1)
		go func() {
			_, err := cacheInitToken(staleSnapshot)
			publisherResult <- err
		}()
		waitForQuotaRaceSignal(t, publisherChecked, "token publisher DB-version precheck")

		type reserveResult struct {
			reserved bool
			err      error
		}
		mutationResult := make(chan reserveResult, 1)
		go func() {
			reserved, err := ReserveTokenQuotaForBilling(token.Id, token.Key, 25, false)
			mutationResult <- reserveResult{reserved: reserved, err: err}
		}()
		waitForQuotaRaceSignal(t, mutationPersisted, "token quota DB mutation")

		server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
		close(releasePublisher)
		assert.ErrorIs(t, <-publisherResult, ErrTokenQuotaCacheStale)
		_, err := cacheGetTokenByKey(token.Key)
		assert.Error(t, err, "the stale token snapshot must be removed by its post-publish version check")

		close(releaseMutation)
		result := <-mutationResult
		require.NoError(t, result.err)
		assert.True(t, result.reserved)
		reloaded := getTokenFromDB(t, token.Id)
		assert.Equal(t, 75, reloaded.RemainQuota)
		assert.Equal(t, 25, reloaded.UsedQuota)
		assert.EqualValues(t, 1, reloaded.QuotaVersion)
		_, err = cacheGetTokenByKey(token.Key)
		assert.Error(t, err, "the successful mutation must publish a second fence after commit")
	})
}

func TestCommittedQuotaMutationDoesNotReturnRetryableErrorWhenFinalFenceFails(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		truncateTables(t)
		resetBatchUpdateTestState(t)
		server := useUserCacheMiniRedis(t)
		user := createReserveTestUser(t, 100)
		setQuotaCacheRaceHooksForTest(t, nil, nil, func() { server.Close() }, nil)

		reserved, err := ReserveUserQuotaForBilling(user.Id, 25, true)
		require.NoError(t, err, "a committed balance mutation must not invite a replay when final cache invalidation fails")
		assert.True(t, reserved)
		assert.Equal(t, 75, getUserQuotaFromDB(t, user.Id))
		assert.EqualValues(t, 1, getUserQuotaVersionFromDB(t, user.Id))
	})

	t.Run("token", func(t *testing.T) {
		truncateTables(t)
		resetBatchUpdateTestState(t)
		server := useUserCacheMiniRedis(t)
		token := createReserveTestToken(t, 100)
		setQuotaCacheRaceHooksForTest(t, nil, nil, nil, func() { server.Close() })

		reserved, err := ReserveTokenQuotaForBilling(token.Id, token.Key, 25, false)
		require.NoError(t, err, "a committed balance mutation must not invite a replay when final cache invalidation fails")
		assert.True(t, reserved)
		reloaded := getTokenFromDB(t, token.Id)
		assert.Equal(t, 75, reloaded.RemainQuota)
		assert.Equal(t, 25, reloaded.UsedQuota)
		assert.EqualValues(t, 1, reloaded.QuotaVersion)
	})
}

func TestCacheHitRejectsSnapshotPublishedBeforeCommitWhenFinalInvalidationFails(t *testing.T) {
	t.Run("user quota", func(t *testing.T) {
		truncateTables(t)
		resetBatchUpdateTestState(t)
		server := useUserCacheMiniRedis(t)
		user := createReserveTestUser(t, 100)
		staleSnapshot := user.ToBaseUser()
		mutationReady := make(chan struct{})
		releaseMutation := make(chan struct{})
		setQuotaCacheBeforeDBMutationHooksForTest(t, func() {
			close(mutationReady)
			<-releaseMutation
		}, nil)
		setQuotaCacheRaceHooksForTest(t, nil, nil, func() { server.Close() }, nil)

		type reserveResult struct {
			reserved bool
			err      error
		}
		mutationResult := make(chan reserveResult, 1)
		go func() {
			reserved, err := ReserveUserQuotaForBilling(user.Id, 25, true)
			mutationResult <- reserveResult{reserved: reserved, err: err}
		}()
		waitForQuotaRaceSignal(t, mutationReady, "user mutation after pre-fence and before database write")

		server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
		require.NoError(t, writeUserCache(staleSnapshot, true))
		cachedBeforeCommit, err := cacheGetUserBase(user.Id)
		require.NoError(t, err)
		assert.Equal(t, 100, cachedBeforeCommit.Quota)

		close(releaseMutation)
		result := <-mutationResult
		require.NoError(t, result.err)
		assert.True(t, result.reserved)
		require.NoError(t, server.Restart())
		assert.Equal(t, 75, getUserQuotaFromDB(t, user.Id))
		assert.EqualValues(t, 1, getUserQuotaVersionFromDB(t, user.Id))

		_, err = cacheGetUserBase(user.Id)
		assert.ErrorIs(t, err, ErrUserQuotaCacheStale)
		fresh, err := GetUserCache(user.Id)
		require.NoError(t, err)
		assert.Equal(t, 75, fresh.Quota)
		assert.EqualValues(t, 1, fresh.QuotaVersion)
	})

	t.Run("disabled token", func(t *testing.T) {
		truncateTables(t)
		resetBatchUpdateTestState(t)
		server := useUserCacheMiniRedis(t)
		token := createReserveTestToken(t, 100)
		staleSnapshot := getTokenFromDB(t, token.Id)
		mutationReady := make(chan struct{})
		releaseMutation := make(chan struct{})
		setQuotaCacheBeforeDBMutationHooksForTest(t, nil, func() {
			close(mutationReady)
			<-releaseMutation
		})
		setQuotaCacheRaceHooksForTest(t, nil, nil, nil, func() { server.Close() })

		token.Status = common.TokenStatusDisabled
		mutationResult := make(chan error, 1)
		go func() { mutationResult <- token.Update() }()
		waitForQuotaRaceSignal(t, mutationReady, "token mutation after pre-fence and before database write")

		server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
		_, err := cacheInitToken(staleSnapshot)
		require.NoError(t, err)
		cachedBeforeCommit, err := cacheGetTokenByKey(token.Key)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusEnabled, cachedBeforeCommit.Status)

		close(releaseMutation)
		require.NoError(t, <-mutationResult)
		require.NoError(t, server.Restart())
		reloaded := getTokenFromDB(t, token.Id)
		assert.Equal(t, common.TokenStatusDisabled, reloaded.Status)
		assert.EqualValues(t, 1, reloaded.QuotaVersion)

		_, err = cacheGetTokenByKey(token.Key)
		assert.ErrorIs(t, err, ErrTokenQuotaCacheStale)
		_, err = ValidateUserToken(token.Key)
		assert.ErrorIs(t, err, ErrTokenInvalid)
	})
}

func TestTokenUpdatePersistsWhenCacheFenceCannotBePublished(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	token := createReserveTestToken(t, 100)
	server.Close()

	token.Name = "must-not-persist"
	token.RemainQuota = 25
	err := token.Update()
	require.NoError(t, err)

	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, "must-not-persist", reloaded.Name)
	assert.Equal(t, 25, reloaded.RemainQuota)
	assert.EqualValues(t, 1, reloaded.QuotaVersion)
	require.NoError(t, server.Restart())
	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, "must-not-persist", cached.Name)
	assert.Equal(t, 25, cached.RemainQuota)
	assert.EqualValues(t, 1, cached.QuotaVersion)
}
