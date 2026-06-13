package cli

import (
	"errors"

	"github.com/tamnd/dockerhub-cli/dockerhub"
)

func isNotFound(err error) bool {
	return errors.Is(err, dockerhub.ErrNotFound)
}
