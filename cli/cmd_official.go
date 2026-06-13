package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) officialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "official",
		Short: "List Docker Official Images",
		Long:  `List images in the Docker Official Images (library) namespace.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(25)
			a.progressf("fetching official images...")
			images, err := a.client.Official(cmd.Context(), n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(images, len(images))
		},
	}
	return cmd
}
