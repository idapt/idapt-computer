package cmdutil

import (
	"errors"
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
)

const AuthHint = `Authentication required.

Sign in with your email and password:
  idapt auth login --email <you> --password <password>

Or paste an API key (get one from https://idapt.ai/settings/api-keys):
  idapt auth login --api-key <uk_…|ak_…|pk_…>

Other ways to provide credentials:
  • export IDAPT_API_KEY=<key>      (env var, useful under sudo / in CI)
  • idapt --api-key <key> <command> (per-invocation flag, overrides the above)

Already signed in? Verify with:
  idapt auth status`

func RequireAuth(f *Factory) error {
	if f == nil {
		return errors.New(AuthHint)
	}
	c, err := f.APIClient()
	if err != nil {
		return err
	}
	if c == nil || c.APIKey() == "" {
		return errors.New(AuthHint)
	}
	return nil
}

func WrapAPIError(err error) error {
	if err == nil {
		return nil
	}
	if api.IsAuthError(err) {
		return fmt.Errorf("%s\n\nServer response: %w", AuthHint, err)
	}
	return err
}
