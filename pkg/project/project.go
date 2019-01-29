package project

import (
	"context"
)

type (
	// Project represents a collaborative shared space owned by an account
	// with three groups for its management.
	Project struct {
		Name         string
		Path         string
		Owner        string
		AdminGroup   string
		ReadersGroup string
		WritersGroup string
	}

	// Manager manipulates the registered projects.
	// TODO(labkode): add CRUD
	Manager interface {
		GetAllProjects(ctx context.Context) ([]*Project, error)
		GetProject(ctx context.Context, name string) (*Project, error)
	}
)
