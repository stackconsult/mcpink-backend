package k8sdeployments

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// CleanupSource removes a cloned workspace after build workflow completion.
func (a *Activities) CleanupSource(ctx context.Context, sourcePath string) error {
	_ = ctx
	if strings.TrimSpace(sourcePath) == "" {
		return nil
	}
	if err := os.RemoveAll(sourcePath); err != nil {
		return fmt.Errorf("cleanup source path %q: %w", sourcePath, err)
	}
	return nil
}

