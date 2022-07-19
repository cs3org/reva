package cache

type ErrNotFound struct{}

func (ErrNotFound) Error() string {
	return "entry in cache not found"
}
