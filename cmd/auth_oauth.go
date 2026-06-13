package cmd

import (
	"context"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/credential"
)

func ensureOAuthAccessToken(
	ctx context.Context,
	baseURL, cliVersion string,
	creds *credential.Credentials,
	credPath string,
	now int64,
) (string, error) {
	if !creds.HasOAuth() {
		return "", nil
	}
	if creds.AccessToken != "" && creds.ExpiresAt-60 > now {
		return creds.AccessToken, nil
	}

	tok, err := api.RefreshOAuthToken(ctx, baseURL, cliVersion, creds.RefreshToken)
	if err != nil {
		return "", err
	}
	creds.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		creds.RefreshToken = tok.RefreshToken
	}
	creds.ExpiresAt = now + int64(tok.ExpiresIn)
	if err := credential.Save(credPath, *creds); err != nil {
		return "", err
	}
	return creds.AccessToken, nil
}
