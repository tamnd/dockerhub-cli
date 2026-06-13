package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) searchCmd() *cobra.Command {
	var (
		onlyOfficial  bool
		onlyAutomated bool
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search Docker Hub images",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := a.effectiveLimit(25)
			a.progressf("searching for %q...", args[0])
			images, err := a.client.Search(cmd.Context(), args[0], n, onlyOfficial, onlyAutomated)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(images, len(images))
		},
	}
	cmd.Flags().BoolVar(&onlyOfficial, "official", false, "show only Docker Official Images")
	cmd.Flags().BoolVar(&onlyAutomated, "automated", false, "show only automated builds")
	return cmd
}
