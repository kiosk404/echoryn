package options

type ModelOptions struct {
	Mode      string        `json:"mode"`
	Providers ModelProvider `json:"providers"`
}

type ModelProvider struct {
	CustomProxy ModelCustomProxy `json:"custom_proxy"`
}

type ModelCustomProxy struct {
	BaseURL    string            `json:"base_url"`
	APIKey     string            `json:"api_key"`
	Api        string            `json:"api"`
	AuthHeader bool              `json:"auth_header"`
	Headers    map[string]string `json:"headers"`
	Models     []Model           `json:"models"`
}

type Model struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Api           string    `json:"api"`
	Reason        string    `json:"reason"`
	Input         []string  `json:"input"`
	Cost          ModelCost `json:"cost"`
	ContextWindow int       `json:"context_window"`
	MaxTokens     int       `json:"max_tokens"`
}

type ModelCost struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
}
