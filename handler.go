package slogtraq

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/traPtitech/go-traq"
)

type Option struct {
	Level slog.Leveler
	// ChannelID is the destination traQ channel ID where logs will be posted.
	ChannelID string
	BotToken  string
}

type Handler struct {
	client messageSender
	opt    Option
	ch     chan string

	groups []string
	attrs  map[string]any
	cur    map[string]any
}

var _ slog.Handler = (*Handler)(nil)

// New creates a new Handler and starts a background goroutine for log transmission.
// Ensure Close() is called when the application shuts down to flush remaining logs.
func New(client *traq.APIClient, option Option) *Handler {
	attrs := make(map[string]any)
	h := &Handler{
		client: &traQClientWrapper{
			client:    client,
			token:     option.BotToken,
			channelID: option.ChannelID,
		},
		opt: option,
		ch:  make(chan string, 10),

		attrs: attrs,
		cur:   attrs,
	}
	go h.sendMessageLoop()
	return h
}

// Close closes the internal log channel and stops the background transmission loop.
// Any pending logs in the channel are flushed to traQ before exiting.
func (h *Handler) Close() {
	close(h.ch)
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opt.Level.Level()
}

func (h *Handler) Handle(_ context.Context, record slog.Record) error {
	h.ch <- h.generateMessageContent(record)
	return nil
}

func (h *Handler) generateMessageContent(r slog.Record) string {
	var content bytes.Buffer

	// level
	content.WriteString(writeLevelStamp(r.Level))
	content.WriteByte(' ')
	// time
	if !r.Time.IsZero() {
		content.WriteString("[")
		content.WriteString(r.Time.Format(time.DateTime))
		content.WriteString("] ")
	}
	// message
	content.WriteString(r.Message)

	// attributes
	attrs, cur := h.extractMap()
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(cur, a)
		return true
	})
	if len(attrs) > 0 {
		content.WriteString("\n```json\n")
		encoder := json.NewEncoder(&content)
		encoder.SetIndent("", "  ")
		encoder.Encode(attrs)
		content.WriteString("```")
	}

	return content.String()
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := h.clone()
	for _, attr := range attrs {
		appendAttr(h2.cur, attr)
	}
	return h2
}

func appendAttr(m map[string]any, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() != slog.KindGroup {
		m[attr.Key] = attr.Value.Any()
		return
	}

	if len(attr.Value.Group()) == 0 {
		return
	}

	if attr.Key == "" {
		// inline group
		maps.Copy(m, convertGroupToMap(attr.Value))
	} else {
		m[attr.Key] = convertGroupToMap(attr.Value)
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	newMap := make(map[string]any)
	h2.cur[name] = newMap
	h2.cur = newMap
	return h2
}

func (h *Handler) clone() *Handler {
	attrs, cur := h.extractMap()
	return &Handler{
		client: h.client,
		opt:    h.opt,
		ch:     h.ch,

		groups: slices.Clip(h.groups),
		attrs:  attrs,
		cur:    cur,
	}
}

func (h *Handler) extractMap() (map[string]any, map[string]any) {
	newAttrs := deepCopyMap(h.attrs)
	newCur := newAttrs
	for _, name := range h.groups {
		if m, ok := newAttrs[name].(map[string]any); ok {
			newCur = m
		} else {
			break
		}
	}
	return newAttrs, newCur
}

func convertGroupToMap(v slog.Value) map[string]any {
	attrs := v.Group()
	if len(attrs) == 0 {
		return nil
	}
	m := make(map[string]any, len(attrs))
	for _, a := range attrs {
		appendAttr(m, a)
	}
	return m
}

func writeLevelStamp(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return ":gear:"
	case slog.LevelInfo:
		return ":information_source:"
	case slog.LevelWarn:
		return ":warning:"
	case slog.LevelError:
		return ":alert:"
	default:
		return ":question:"
	}
}

func (h *Handler) sendMessageLoop() {
	var builder strings.Builder
	ticker := time.NewTicker(time.Second)

	for {
		select {
		case msg, ok := <-h.ch:
			if !ok {
				h.flush(&builder)
				return
			}
			if builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(msg)
		case <-ticker.C:
			h.flush(&builder)
		}
	}
}

func (h *Handler) flush(b *strings.Builder) {
	if b.Len() == 0 {
		return
	}
	_ = h.client.send(context.Background(), b.String())
	b.Reset()
}

func deepCopyMap(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	for k, v := range m {
		if vm, ok := v.(map[string]any); ok {
			cp[k] = deepCopyMap(vm)
		} else {
			cp[k] = v
		}
	}
	return cp
}

// messageSender defines the behavior of a message transmission (abstracted for testing).
type messageSender interface {
	send(ctx context.Context, content string) error
}

type traQClientWrapper struct {
	client    *traq.APIClient
	token     string
	channelID string
}

func (c *traQClientWrapper) send(ctx context.Context, content string) error {
	ctx = context.WithValue(ctx, traq.ContextAccessToken, c.token)
	_, _, err := c.client.MessageAPI.
		PostMessage(ctx, c.channelID).
		PostMessageRequest(traq.PostMessageRequest{Content: content}).
		Execute()
	return err
}
