package bugcrowd

import (
	"context"
	"strings"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
)

type Poller struct{ token string }

// NewPollerFromToken uses an existing _bugcrowd_session token.
func NewPollerFromToken(token string) *Poller { return &Poller{token: token} }

func (p *Poller) Name() string { return "bc" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cfg.Token != "" {
		p.token = cfg.Token
		return nil
	}
	if cfg.Email != "" && cfg.Password != "" && cfg.OtpSecret != "" {
		tok, err := Login(ctx, cfg.Email, cfg.Password, cfg.OtpSecret, cfg.Proxy)
		if err != nil {
			return err
		}
		p.token = tok
		return nil
	}
	return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	// Reuse existing listing: bbpOnly controls category, pvtOnly controls open/private
	handles, err := GetProgramHandles(ctx, p.token, "bug_bounty", opts.PrivateOnly)
	if err != nil {
		return nil, err
	}
	// Optionally include VDP if not bbpOnly. A VDP listing failure used to
	// be swallowed, so a WAF/HTML page looked like "no VDP programs".
	if !opts.BountyOnly {
		vdp, err := GetProgramHandles(ctx, p.token, "vdp", opts.PrivateOnly)
		if err != nil {
			return nil, err
		}
		handles = append(handles, vdp...)
	}
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	cats := opts.Categories
	if cats == "" {
		cats = "all"
	}
	pd, err := GetProgramScope(ctx, handle, cats, p.token)
	if err != nil {
		return scope.ProgramData{Url: strings.TrimPrefix(handle, "/")}, err
	}
	return pd, nil
}
