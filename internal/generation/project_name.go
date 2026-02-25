package generation

import "context"

type ProjectNameGenerator interface {
	GenerateProjectName(ctx context.Context, goalPrompt string) (string, error)
}
