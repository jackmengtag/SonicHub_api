package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/core"
)

type Worker struct {
	Core *core.Core
}

func (w *Worker) Run(ctx context.Context) {
	_ = w.Core.Store.RequeueStale(ctx)
	slog.Info("generation worker started")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("generation worker stopped")
			return
		case <-ticker.C:
			w.processOne(ctx)
		}
	}
}

func (w *Worker) processOne(parent context.Context) {
	gen, err := w.Core.Store.ClaimGeneration(parent)
	if err != nil {
		slog.Error("claim generation", "error", err)
		return
	}
	if gen == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	if err := w.Core.ProcessGeneration(ctx, gen); err != nil {
		slog.Error("process generation", "id", gen.ID, "error", err)
		_ = w.Core.Store.FailGeneration(ctx, gen.ID, "generation failed")
		if releaseErr := w.Core.Store.Release(ctx, gen.ID, gen.TenantID); releaseErr != nil {
			slog.Error("release credit", "id", gen.ID, "error", releaseErr)
		}
		_ = w.Core.Store.Notify(ctx, gen.UserID, "credit", "", "生成失败，积分已退还",
			"任务 "+gen.ID+" 失败，已释放冻结积分", "generation", gen.ID, "fail:"+gen.ID)
		return
	}
	slog.Info("generation finished", "id", gen.ID, "work", gen.WorkID)
}
