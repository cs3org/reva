package cache

type Cache interface {
	Get(file string, width, height int) ([]byte, error)
	Set(file string, width, height int, data []byte) error
}

type noCache struct{}

func NewNoCache() Cache {
	return noCache{}
}

func (noCache) Get(_ string, _, _ int) ([]byte, error) {
	return nil, ErrNotFound{}
}

func (noCache) Set(_ string, _, _ int, _ []byte) error {
	return nil
}
