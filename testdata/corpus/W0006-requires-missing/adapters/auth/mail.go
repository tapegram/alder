package auth

import "os"

func smtp() string {
	if os.Getenv("ALDER_DEV") == "1" {
		return ""
	}
	_, _ = os.LookupEnv("STRIPE_KEY")
	return os.Getenv("SMTP_URL")
}
