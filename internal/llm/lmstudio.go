package llm

type LMStudioProvider struct {
	*OpenAIProvider
}

func NewLMStudioProvider(cfg Config) (*LMStudioProvider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL(ProviderLMStudio)
	}

	provider, err := NewOpenAIProvider(cfg)
	if err != nil {
		return nil, err
	}

	return &LMStudioProvider{OpenAIProvider: provider}, nil
}

func (p *LMStudioProvider) Name() string {
	return "LM Studio"
}

func (p *LMStudioProvider) Type() ProviderType {
	return ProviderLMStudio
}
