package keyring_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/subosito/cincai/ingress/keyring"
)

func TestBudgetRollingWindow(t *testing.T) {
	ks := keyring.NewMemoryStore()
	secret, id, err := ks.Create(context.Background(), "budgeted", keyring.KindStatic, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.SetBudget(context.Background(), id, 100, time.Hour); err != nil {
		t.Fatal(err)
	}
	p, err := ks.Verify(context.Background(), secret)
	if err != nil {
		t.Fatal(err)
	}
	if !p.HasBudget() || p.BudgetMaxTokens != 100 {
		t.Fatalf("principal budget=%+v", p)
	}

	st, err := keyring.CheckBudget(context.Background(), ks, p)
	if err != nil {
		t.Fatalf("under limit: %v", err)
	}
	if st.UsedTokens != 0 {
		t.Fatalf("used=%d", st.UsedTokens)
	}

	if err := keyring.RecordBudgetTokens(context.Background(), ks, p, 60, 30); err != nil {
		t.Fatal(err)
	}
	st, err = keyring.CheckBudget(context.Background(), ks, p)
	if err != nil {
		t.Fatalf("still under: %v used=%d", err, st.UsedTokens)
	}
	if st.UsedTokens != 90 {
		t.Fatalf("used=%d want 90", st.UsedTokens)
	}

	if err := keyring.RecordBudgetTokens(context.Background(), ks, p, 20, 0); err != nil {
		t.Fatal(err)
	}
	st, err = keyring.CheckBudget(context.Background(), ks, p)
	if !errors.Is(err, keyring.ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v st=%+v", err, st)
	}
	if st.UsedTokens < 100 {
		t.Fatalf("used=%d", st.UsedTokens)
	}
}

func TestBudgetClear(t *testing.T) {
	ks := keyring.NewMemoryStore()
	_, id, err := ks.Create(context.Background(), "x", keyring.KindStatic, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.SetBudget(context.Background(), id, 10, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := ks.SetBudget(context.Background(), id, 0, 0); err != nil {
		t.Fatal(err)
	}
	keys, err := ks.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if k.ID == id && k.BudgetMaxTokens != 0 {
			t.Fatalf("budget not cleared: %+v", k)
		}
	}
}

func TestBudgetUnlimitedSkipsCheck(t *testing.T) {
	ks := keyring.NewMemoryStore()
	secret, _, err := ks.Create(context.Background(), "free", keyring.KindStatic, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ks.Verify(context.Background(), secret)
	if err != nil {
		t.Fatal(err)
	}
	if p.HasBudget() {
		t.Fatal("expected unlimited")
	}
	if _, err := keyring.CheckBudget(context.Background(), ks, p); err != nil {
		t.Fatal(err)
	}
}
