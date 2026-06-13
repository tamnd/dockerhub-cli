package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/dockerhub-cli/dockerhub"
)

func (a *App) imageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "image <name>",
		Short: "Show metadata for a Docker Hub image",
		Long: `Show metadata for a single Docker Hub image.

name can be a plain image name like "nginx" (official) or "user/repo".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			a.progressf("fetching image %q...", name)
			img, err := a.client.ImageDetail(cmd.Context(), name)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.render([]dockerhub.Image{img})
		},
	}
}
