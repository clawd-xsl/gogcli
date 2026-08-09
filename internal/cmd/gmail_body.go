package cmd

import (
	"context"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/gmailcontent"
)

func gmailMessageBodyText(ctx context.Context, service *gmail.Service, message *gmail.Message) (string, error) {
	if message == nil {
		return "", nil
	}
	return gmailcontent.BestBodyTextWithAttachments(ctx, message.Payload, gmailBodyLoader(service, message.Id))
}

func gmailMessageBodyForDisplay(ctx context.Context, service *gmail.Service, message *gmail.Message) (string, bool, error) {
	if message == nil {
		return "", false, nil
	}
	return gmailcontent.BestBodyForDisplayWithAttachments(ctx, message.Payload, gmailBodyLoader(service, message.Id))
}

func gmailBodyLoader(service *gmail.Service, messageID string) gmailcontent.AttachmentBodyLoader {
	return func(ctx context.Context, attachmentID string) ([]byte, error) {
		return fetchAttachmentBytes(ctx, service, messageID, attachmentID)
	}
}
