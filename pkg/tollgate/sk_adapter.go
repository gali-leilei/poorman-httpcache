package tollgate

import (
	"context"
	"fmt"
)

// SecretKeyAdapter is an adapter that consumes and balances a secret key.
type SecretKeyAdapter struct {
	sk string
}

func NewSecretKeyAdapter(sk string) Adapter {
	return &SecretKeyAdapter{sk: sk}
}

func (a *SecretKeyAdapter) Consume(ctx context.Context, ticket string) (int, error) {
	if ticket != a.sk {
		return 0, fmt.Errorf("invalid ticket")
	}
	return 1, nil
}

func (a *SecretKeyAdapter) Balance(ctx context.Context, ticket string) (int, error) {
	if ticket != a.sk {
		return 0, fmt.Errorf("invalid ticket")
	}
	return 1, nil
}
