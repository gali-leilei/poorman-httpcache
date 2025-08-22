package tollgate

import (
	"context"
	"fmt"
)

type EnvAdapter struct {
	envKey string
}

func NewEnvAdapter(envKey string) Adapter {
	return &EnvAdapter{envKey: envKey}
}

func (a *EnvAdapter) Consume(ctx context.Context, ticket string) (int, error) {
	if ticket != a.envKey {
		return 0, fmt.Errorf("invalid ticket")
	}
	return 1, nil
}

func (a *EnvAdapter) Balance(ctx context.Context, ticket string) (int, error) {
	if ticket != a.envKey {
		return 0, fmt.Errorf("invalid ticket")
	}
	return 1, nil
}
