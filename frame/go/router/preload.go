package router

type PreloadConfig struct {
	pages        []string
	dynamicPages []string
	rootPath     string
	whileOrBlack bool
}

func NewPreloadConfig(rootPath string, whileOrBlack bool) *PreloadConfig {
	return &PreloadConfig{
		pages:        []string{},
		dynamicPages: []string{},
		rootPath:     rootPath,
		whileOrBlack: whileOrBlack,
	}
}
