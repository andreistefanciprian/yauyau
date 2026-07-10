package mailer

import (
	"context"
	"log"
)

// Mailer sends magic-link emails. auth-service owns when links are issued;
// this package owns only delivery mechanics.
type Mailer interface {
	SendMagicLink(ctx context.Context, email, link string) error
}

// Stdout logs magic links for local development, preserving the PR6/PR7
// workflow where developers copy the link from docker compose logs.
type Stdout struct{}

func (Stdout) SendMagicLink(_ context.Context, email, link string) error {
	log.Printf("magic link for %s: %s", email, link)
	return nil
}
