package roon

func applyConfigDefaults(cfg Config) Config {
	if cfg.ExtensionID == "" {
		cfg.ExtensionID = "com.lukaslosche.roon-cover"
	}
	if cfg.DisplayName == "" {
		cfg.DisplayName = "roon-cover"
	}
	if cfg.DisplayVersion == "" {
		cfg.DisplayVersion = "dev"
	}
	if cfg.Publisher == "" {
		cfg.Publisher = "roon-cover"
	}
	if cfg.Email == "" {
		cfg.Email = "n/a"
	}
	if cfg.Website == "" {
		cfg.Website = ""
	}
	return cfg
}
