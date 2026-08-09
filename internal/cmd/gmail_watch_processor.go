package cmd

import (
	"context"

	"github.com/openclaw/gogcli/internal/gmailwatch"
)

func (s *gmailWatchServer) watchProcessor() *gmailwatch.Processor {
	return s.watchProcessorFor(s.cfg.Account, s.store)
}

func (s *gmailWatchServer) watchProcessorFor(account string, store *gmailWatchStore) *gmailwatch.Processor {
	processor := &gmailwatch.Processor{
		Config: gmailwatch.ProcessorConfig{
			Account:      account,
			HistoryMax:   s.cfg.HistoryMax,
			ResyncMax:    s.cfg.ResyncMax,
			FetchDelay:   s.cfg.FetchDelay,
			HistoryTypes: s.cfg.HistoryTypes,
			Verbose:      s.cfg.VerboseOutput,
		},
		Repository: store,
		NewSource: func(ctx context.Context) (gmailwatch.Source, error) {
			service, err := s.newService(ctx, account)
			if err != nil {
				return nil, err
			}

			return newGmailWatchSource(service, s.cfg, s.excludeLabelIDs, s.logf), nil
		},
		Now:                 s.currentTime,
		Sleep:               s.sleep,
		IsStaleHistoryError: isStaleHistoryError,
		IsTerminalAuthError: isTerminalGmailAuthError,
		RateLimitUntil:      gmailWatchRateLimitUntil,
		Logf:                s.logf,
		Warnf:               s.warnf,
	}
	if s.cfg.HookURL != "" {
		processor.Deliver = s.deliverHook
	}

	return processor
}

func (s *gmailWatchServer) handlePush(ctx context.Context, payload gmailPushPayload) (*gmailHookPayload, error) {
	return s.watchProcessor().Handle(ctx, gmailwatch.Notification{
		HistoryID: payload.HistoryID,
		MessageID: payload.MessageID,
	})
}
