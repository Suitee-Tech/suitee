package service

// CtxKey is the canonical type for context.WithValue keys used across
// toca-comm services and handlers. Using a custom type avoids collisions
// with other packages' string-typed keys (staticcheck SA1029).
type CtxKey string

const (
	// CtxSenderName carries the human-readable name of the message sender.
	CtxSenderName CtxKey = "sender_name"
	// CtxSenderAvatarURL carries the avatar URL of the message sender.
	CtxSenderAvatarURL CtxKey = "sender_avatar_url"
	// CtxReplyToID carries the ID of the message being replied to.
	CtxReplyToID CtxKey = "reply_to_id"
	// CtxContentType carries the message content type ("text", "system", etc.).
	CtxContentType CtxKey = "content_type"
	// CtxAttachments carries a []message.Attachment to be appended to the message.
	CtxAttachments CtxKey = "attachments"
)
