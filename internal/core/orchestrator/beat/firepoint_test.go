// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package beat

import (
	"testing"
	"time"
)

func TestPreviousFirePoint_Every(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 17, 42, 0, time.UTC)
	got, err := PreviousFirePoint(now, "", "15m")
	if err != nil {
		t.Fatal(err)
	}
	// 12:15 UTC is the floor of 12:17:42 within a 15-minute window.
	want := time.Date(2026, 5, 30, 12, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPreviousFirePoint_EveryHour(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 17, 42, 0, time.UTC)
	got, err := PreviousFirePoint(now, "", "1h")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPreviousFirePoint_Cron(t *testing.T) {
	// Every day at 03:00 UTC.
	now := time.Date(2026, 5, 30, 12, 17, 42, 0, time.UTC)
	got, err := PreviousFirePoint(now, "0 3 * * *", "")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 5, 30, 3, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPreviousFirePoint_RejectsBoth(t *testing.T) {
	if _, err := PreviousFirePoint(time.Now(), "0 3 * * *", "1h"); err == nil {
		t.Fatal("want error when both cron + every set")
	}
	if _, err := PreviousFirePoint(time.Now(), "", ""); err == nil {
		t.Fatal("want error when neither cron nor every set")
	}
}

func TestPreviousFirePoint_BadEvery(t *testing.T) {
	if _, err := PreviousFirePoint(time.Now(), "", "30x"); err == nil {
		t.Fatal("want error on bad every")
	}
}

func TestPreviousFirePoint_BadCron(t *testing.T) {
	if _, err := PreviousFirePoint(time.Now(), "not-a-cron", ""); err == nil {
		t.Fatal("want error on bad cron")
	}
}
