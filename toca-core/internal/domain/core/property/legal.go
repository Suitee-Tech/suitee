package property

type Legal struct {
	RegistrationNumber string `json:"registration_number,omitempty"`
	NotaryOffice       string `json:"notary_office,omitempty"`
	DeedStatus         string `json:"deed_status,omitempty"`
}

func (l *Legal) Validate() error {
	return validate.Struct(l)
}
