package model

import "sync"

// These hooks provide deterministic race seams for quota-cache tests. They
// are nil in production and protected because cache fills and quota mutations
// are concurrent by design.
var quotaCacheRaceHooks struct {
	sync.RWMutex
	userAfterVersionCheck  func()
	tokenAfterVersionCheck func()
	userBeforeDBMutation   func()
	tokenBeforeDBMutation  func()
	userAfterDBMutation    func()
	tokenAfterDBMutation   func()
}

func runQuotaCacheRaceHook(hook func()) {
	if hook != nil {
		hook()
	}
}

func userQuotaCacheAfterVersionCheckHook() func() {
	quotaCacheRaceHooks.RLock()
	defer quotaCacheRaceHooks.RUnlock()
	return quotaCacheRaceHooks.userAfterVersionCheck
}

func tokenQuotaCacheAfterVersionCheckHook() func() {
	quotaCacheRaceHooks.RLock()
	defer quotaCacheRaceHooks.RUnlock()
	return quotaCacheRaceHooks.tokenAfterVersionCheck
}

func userQuotaCacheBeforeDBMutationHook() func() {
	quotaCacheRaceHooks.RLock()
	defer quotaCacheRaceHooks.RUnlock()
	return quotaCacheRaceHooks.userBeforeDBMutation
}

func tokenQuotaCacheBeforeDBMutationHook() func() {
	quotaCacheRaceHooks.RLock()
	defer quotaCacheRaceHooks.RUnlock()
	return quotaCacheRaceHooks.tokenBeforeDBMutation
}

func userQuotaCacheAfterDBMutationHook() func() {
	quotaCacheRaceHooks.RLock()
	defer quotaCacheRaceHooks.RUnlock()
	return quotaCacheRaceHooks.userAfterDBMutation
}

func tokenQuotaCacheAfterDBMutationHook() func() {
	quotaCacheRaceHooks.RLock()
	defer quotaCacheRaceHooks.RUnlock()
	return quotaCacheRaceHooks.tokenAfterDBMutation
}
