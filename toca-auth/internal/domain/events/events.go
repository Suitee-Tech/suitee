package events

type UserRegisteredEvent struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	TenantID  string `json:"tenant_id"`
	AppURL    string `json:"app_url"`
}

func (e UserRegisteredEvent) Name() string         { return "auth.user.registered" }
func (e UserRegisteredEvent) Payload() interface{} { return e }

type PasswordResetRequestedEvent struct {
	Email    string `json:"email"`
	Token    string `json:"token"`
	ResetURL string `json:"reset_url"`
}

func (e PasswordResetRequestedEvent) Name() string         { return "auth.password_reset.requested" }
func (e PasswordResetRequestedEvent) Payload() interface{} { return e }

type InvitationCreatedEvent struct {
	Email       string `json:"email"`
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
	Role        string `json:"role"`
	Token       string `json:"token"`
	InviteURL   string `json:"invite_url"`
}

func (e InvitationCreatedEvent) Name() string         { return "auth.invitation.created" }
func (e InvitationCreatedEvent) Payload() interface{} { return e }

type OTPRequestedEvent struct {
	Contact string `json:"contact"`
	Method  string `json:"method"`
	Code    string `json:"code"`
}

func (e OTPRequestedEvent) Name() string         { return "auth.otp.requested" }
func (e OTPRequestedEvent) Payload() interface{} { return e }

func (e InvitationCreatedEvent) EventID() string {
	return ""
}

func (e PasswordResetRequestedEvent) EventID() string {
	return ""
}

func (e UserRegisteredEvent) EventID() string {
	return ""
}

func (e OTPRequestedEvent) EventID() string {
	return ""
}
