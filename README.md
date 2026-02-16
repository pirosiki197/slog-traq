# slog-traq

slog Handler for [traQ](https://github.com/traPtitech/traQ)

## Example
```go
	client := traq.NewAPIClient(traq.NewConfiguration())

	logger := slog.New(slog.NewMultiHandler(
		slog.NewTextHandler(os.Stdout, nil),
		slogtraq.New(client, slogtraq.Option{
			Level:     slog.LevelError, // Filter for critical notifications
			ChannelID: os.Getenv("SLOG_TRAQ_CHANNEL_ID"),
			BotToken:  os.Getenv("TRAQ_BOT_TOKEN"),
		}),
	))

	// Only goes to stdout
	logger.Info("message")
	// Fanned out to both stdout and traQ
	logger.Error("error!")
```
