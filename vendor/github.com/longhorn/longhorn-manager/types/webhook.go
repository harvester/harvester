package types

const (
	WebhookTypeConversion = "conversion"
	WebhookTypeAdmission  = "admission"

	ValidatingWebhookName        = "longhorn-webhook-validator"
	MutatingWebhookName          = "longhorn-webhook-mutator"
	ConversionWebhookServiceName = "longhorn-conversion-webhook"
	AdmissionWebhookServiceName  = "longhorn-admission-webhook"

	CaName   = "longhorn-webhook-ca"
	CertName = "longhorn-webhook-tls"
)
