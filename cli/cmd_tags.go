package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) tagsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tags <image>",
		Short: "List tags for a Docker Hub image",
		Long: `List tags for a Docker Hub image.

image can be a plain name like "nginx" (official) or "user/repo".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			n := a.effectiveLimit(25)
			a.progressf("fetching tags for %q...", name)
			tags, err := a.client.Tags(cmd.Context(), name, n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(tags, len(tags))
		},
	}
	return cmd
}
