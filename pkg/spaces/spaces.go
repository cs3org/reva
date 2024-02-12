package spaces

type SpaceType string

const (
	SpaceTypeHome    SpaceType = "personal"
	SpaceTypeProject SpaceType = "project"
)

func (t SpaceType) AsString() string { return string(t) }
