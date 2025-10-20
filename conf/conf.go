package conf

import (
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("github.com/KiClover/orzfrp")

var (
	Namespace  = "orzfrp"
	Version    = "v0.0.1"
	InstanceId = uuid.NewString()
)
