package service

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var grossProfitWorkerOnce sync.Once

func StartGrossProfitRollupWorker() {
	grossProfitWorkerOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			owner := common.NewRequestId()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for range ticker.C {
				now := time.Now()
				acquired, err := model.AcquireSystemTaskLock("gross_profit_rollup", "gross-profit-rollup", owner, now.Add(30*time.Second).Unix())
				if err != nil {
					common.SysError("gross profit worker lease: " + err.Error())
					continue
				}
				if !acquired {
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				err = model.ReconcileGrossProfit(ctx, 500, now)
				cancel()
				if err != nil {
					common.SysError("gross profit reconciliation: " + err.Error())
				}
				if err := model.ReleaseSystemTaskLock("gross-profit-rollup", owner); err != nil {
					common.SysError("gross profit worker release: " + err.Error())
				}
			}
		}()
	})
}
