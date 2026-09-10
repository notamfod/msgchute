package sender

import "sort"

// Transport describes a configured provider instance, without its credentials.
type Transport struct {
	Code      string `json:"code"`
	Provider  string `json:"provider"`
	Available bool   `json:"available"`
	Sender    string `json:"sender,omitempty"`
}

func (c *Config) Transports() []Transport {
	items := make([]Transport, 0, len(c.Providers))
	for code, config := range c.Providers {
		if config == nil {
			continue
		}
		item := Transport{Code: code, Provider: config.Provider, Available: true}
		switch config.Provider {
		case "smtp":
			item.Sender, _ = config.Params["from"].(string)
		case "smsc":
			item.Sender, _ = config.Params["sender"].(string)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Code < items[j].Code })
	return items
}
