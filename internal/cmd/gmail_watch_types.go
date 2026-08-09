package cmd

import (
	"strings"
	"time"

	"github.com/openclaw/gogcli/internal/gmailwatch"
)

const (
	defaultWatchPath             = "/gmail-pubsub"
	defaultWatchPort             = 8788
	defaultHookMaxBytes          = 20000
	defaultHistoryMaxResults     = 100
	defaultHistoryResyncMax      = 10
	defaultHistoryFetchDelay     = 3 * time.Second
	defaultPushBodyLimitBytes    = 1024 * 1024
	defaultHookRequestTimeoutSec = 10
)

type gmailWatchServeConfig struct {
	Account            string
	Accounts           []string
	Bind               string
	Port               int
	Path               string
	VerifyOIDC         bool
	OIDCEmail          string
	OIDCAudience       string
	SharedToken        string
	HookURL            string
	HookToken          string
	IncludeBody        bool
	MaxBodyBytes       int
	MaxPayloadBytes    int
	MaxPayloadMessages int
	ExcludeLabels      []string
	HistoryMax         int64
	ResyncMax          int64
	FetchDelay         time.Duration
	HistoryTypes       []string
	HookTimeout        time.Duration
	DateLocation       *time.Location
	PersistHook        bool
	AllowNoHook        bool
	VerboseOutput      bool
}

func configuredWatchAccounts(primary string, additional []string) []string {
	accounts := make([]string, 0, len(additional)+1)
	seen := make(map[string]struct{}, len(additional)+1)
	add := func(account string) {
		trimmed := strings.TrimSpace(account)
		key := strings.ToLower(trimmed)
		if trimmed == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		accounts = append(accounts, trimmed)
	}
	add(primary)
	for _, raw := range additional {
		for _, account := range strings.Split(raw, ",") {
			add(account)
		}
	}
	return accounts
}

var gmailHistoryTypes = []string{
	"messageAdded",
	"messageDeleted",
	"labelAdded",
	"labelRemoved",
}

var gmailHistoryTypesHelp = strings.Join(gmailHistoryTypes, ",")

var gmailHistoryTypeAliases = func() map[string]string {
	aliases := make(map[string]string, len(gmailHistoryTypes)+4)
	for _, historyType := range gmailHistoryTypes {
		aliases[strings.ToLower(historyType)] = historyType
	}
	aliases["messagesadded"] = "messageAdded"
	aliases["messagesdeleted"] = "messageDeleted"
	aliases["labelsadded"] = "labelAdded"
	aliases["labelsremoved"] = "labelRemoved"
	return aliases
}()

func parseHistoryTypes(values []string) ([]string, error) {
	if len(values) == 0 {
		// Default to messageAdded for backward compatibility.
		// Previously this was hardcoded; returning nil would fetch ALL types.
		return []string{"messageAdded"}, nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			normalized, ok := gmailHistoryTypeAliases[strings.ToLower(trimmed)]
			if !ok {
				return nil, usage("--history-types must be one of " + gmailHistoryTypesHelp)
			}
			if _, exists := seen[normalized]; exists {
				continue
			}
			seen[normalized] = struct{}{}
			out = append(out, normalized)
		}
	}
	if len(out) == 0 {
		return nil, usage("--history-types must include at least one value")
	}
	return out, nil
}

type (
	pubsubPushEnvelope = gmailwatch.PushEnvelope
	gmailPushPayload   = gmailwatch.PushPayload
	gmailHookPayload   = gmailwatch.Payload
)
