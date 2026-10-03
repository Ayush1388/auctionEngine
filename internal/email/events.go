package email

// EventTypeActivationEmail and ActivationEmailEvent are the contract between
// the user package (which enqueues the event) and this package (which
// consumes it). They are defined once, here, so the two sides can't drift.
const EventTypeActivationEmail = "email.activation"

type ActivationEmailEvent struct {
	To              string `json:"to"`
	ActivationToken string `json:"activation_token"`
}
