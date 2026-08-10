package stub

import (
	"context"

	"github.com/f1nniboy/lrcmux/internal/lyrics"
	"github.com/f1nniboy/lrcmux/internal/providers"
)

type Provider struct {
	providers.Common
}

func (p *Provider) ID() string                 { return "stub" }
func (p *Provider) Name() string               { return "Stub" }
func (p *Provider) Desc() string               { return "For testing only" }
func (p *Provider) MaxLevel() lyrics.SyncLevel { return lyrics.SyncWord }

func (p *Provider) Search(_ context.Context, _ lyrics.Query) (*lyrics.Result, error) {
	return &lyrics.Result{
		Lines: []lyrics.Line{
			{Text: "Never gonna give you up", StartMs: 0, EndMs: 1000},
			{Text: "Never gonna let you down", StartMs: 2000, EndMs: 3000},
			{Text: "Never gonna run around and desert you", StartMs: 4000, EndMs: 5000},
		},
		SyncLevel: lyrics.SyncWord,
	}, nil
}
