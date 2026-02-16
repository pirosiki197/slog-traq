package slogtraq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type mockSender struct {
	w    io.Writer
	sent int
}

func newMockSender(w io.Writer) *mockSender {
	return &mockSender{
		w:    w,
		sent: 0,
	}
}

var _ messageSender = (*mockSender)(nil)

func (s *mockSender) send(_ context.Context, content string) error {
	s.w.Write([]byte(content))
	s.sent++
	return nil
}

func TestBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		mock := newMockSender(buf)
		h := New(nil, Option{Level: slog.LevelInfo})
		h.client = mock
		defer h.Close()
		logger := slog.New(h)

		const logCount = 5
		for i := range logCount {
			logger.Info(fmt.Sprintf("message %d", i))
		}

		time.Sleep(1 * time.Second)
		synctest.Wait()

		if mock.sent != 1 {
			t.Errorf("expected 1 send call, but got %d", mock.sent)
		}

		got := strings.Split(buf.String(), "\n")
		if len(got) != logCount {
			t.Errorf("expected %d lines, but got %d", logCount, len(got))
		}

		for _, line := range got {
			t.Log(line)
		}
	})
}

func TestLogHeader(t *testing.T) {

	tests := []struct {
		level slog.Level
		stamp string
	}{
		{slog.LevelInfo, ":information_source:"},
		{slog.LevelError, ":alert:"},
		{slog.LevelDebug, ":gear:"},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				buf := new(bytes.Buffer)
				mock := newMockSender(buf)
				h := New(nil, Option{Level: slog.LevelDebug})
				h.client = mock
				defer h.Close()

				timestamp := time.Date(2009, 2, 13, 23, 31, 30, 0, time.UTC)
				record := slog.NewRecord(timestamp, tt.level, "message", 0)
				h.Handle(context.Background(), record)

				time.Sleep(1 * time.Second)
				synctest.Wait()

				header, _, _ := strings.Cut(buf.String(), "\n")
				expected := fmt.Sprintf("%s [%s] message", tt.stamp, timestamp.Format(time.DateTime))
				if header != expected {
					t.Errorf("expected: %s, but got: %s", expected, header)
				}
			})
		})
	}
}

func TestAttributes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := new(bytes.Buffer)
		mock := newMockSender(buf)

		h := New(nil, Option{Level: slog.LevelDebug})
		h.client = mock
		defer h.Close()

		logger := slog.New(h).
			With("version", "1.0.0")

		logger.Info("op success",
			slog.String("details", "all green"),
			slog.Int("count", 42),
			slog.Group("user", slog.String("name", "gopher")))

		time.Sleep(1 * time.Second)
		synctest.Wait()

		content := buf.String()
		_, block, _ := strings.Cut(content, "```json\n")
		jsonPart := strings.TrimSuffix(block, "\n```")

		expectedJson := `{
  "version": "1.0.0",
  "details": "all green",
  "count": 42,
  "user": {
    "name": "gopher"
  }
}`
		expected := make(map[string]any)
		json.Unmarshal([]byte(expectedJson), &expected)

		got := make(map[string]any)
		if err := json.Unmarshal([]byte(jsonPart), &got); err != nil {
			t.Fatal(err)
		}

		if !compareMap(got, expected) {
			t.Errorf("expected: %v, but got: %v", expected, got)
		}
	})
}

func compareMap(m1, m2 map[string]any) bool {
	if len(m1) != len(m2) {
		return false
	}

	res := true
	for key, value1 := range m1 {
		value2 := m2[key]
		mm1, ok1 := value1.(map[string]any)
		mm2, ok2 := value2.(map[string]any)
		if ok1 && ok2 {
			res = res && compareMap(mm1, mm2)
		} else {
			res = res && value1 == value2
		}
	}
	return res
}
