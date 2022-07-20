package cache

type Cache interface {
	Get(file, etag string, width, height int) ([]byte, error)
	Set(file, etag string, width, height int, data []byte) error
}

type noCache struct{}

func NewNoCache() Cache {
	return noCache{}
}

func (noCache) Get(_, _ string, _, _ int) ([]byte, error) {
	return nil, ErrNotFound{}
}

func (noCache) Set(_, _ string, _, _ int, _ []byte) error {
	return nil
}
