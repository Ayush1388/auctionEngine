package email

const EventTypeActivationEmail = "email.activation"

type ActivationEmailEvent struct {
	To              string `json:"to"`
	ActivationToken string `json:"activation_token"`
}
