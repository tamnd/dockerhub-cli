package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) userCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user <username>",
		Short: "List public repositories for a Docker Hub user or organisation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			n := a.effectiveLimit(25)
			a.progressf("fetching repos for %q...", username)
			images, err := a.client.UserRepos(cmd.Context(), username, n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(images, len(images))
		},
	}
	return cmd
}
