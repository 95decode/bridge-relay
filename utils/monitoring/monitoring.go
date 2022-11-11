package monitoring

import (
	"log"

	"github.com/getsentry/sentry-go"
)

func init() {
	err := sentry.Init(sentry.ClientOptions{
		// Todo : Separate Dsn into env file
		Dsn: "",
	})

	if err != nil {
		log.Fatalf("Sentry initialize failed: %s", err)
	}
}

func Message(msg string) {
	sentry.CaptureMessage(msg)
}

func Error(err error) {
	sentry.CaptureException(err)
}
