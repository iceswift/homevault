package engine

import (
	"context"
	"homevault/internal/config"
	"testing"
)

func TestRecoveryOnlyRefusesMutationBeforePreparation(t *testing.T) {
	e := fixture(t)
	e.cfg.RecoveryOnly = true
	e.run = func(context.Context, []string, []string, bool) ([]byte, error) {
		t.Fatal("mutation reached child execution")
		return nil, nil
	}
	ctx := context.Background()
	for _, call := range []func() error{
		func() error { return e.Init(ctx) }, func() error { return e.Backup(ctx, "photos") }, func() error { return e.RunBackupJob(ctx, "photos") },
		func() error { return e.RunCapturedJob(ctx, "photos", CapturePlan{}) },
		func() error { _, err := e.ApplyRetention(ctx, "photos", RetentionReview{}, ""); return err },
		func() error { _, err := e.ApplyPrune(ctx, PruneReview{}, ""); return err },
		func() error {
			return e.runBackupJob(ctx, "photos", func(context.Context, config.Project) (config.Project, func() error, error) { panic("unreachable") })
		},
	} {
		if err := call(); err == nil {
			t.Fatal("mutation allowed in recovery-only mode")
		}
	}
	for _, cmd := range []string{"init", "backup", "forget", "prune", "repair", "rewrite", "migrate"} {
		if _, err := e.command(ctx, "guard test", false, cmd); err == nil {
			t.Fatal("repository write command allowed", cmd)
		}
	}
}
