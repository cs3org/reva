package ocdav

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/config"
)

// Validator validates strings
type Validator func(string) error

// ValidatorsFromConfig returns the configured Validators
func ValidatorsFromConfig(c *config.Config) []Validator {
	// we always want to exclude empty names
	vals := []Validator{notEmpty()}

	// forbidden characters
	vals = append(vals, doesNotContain(c.NameValidation.InvalidChars))

	// max length
	vals = append(vals, isShorterThan(c.NameValidation.MaxLength))

	vals = append(vals, invalidNames(c.NameValidation.InvalidNames))

	return vals
}

// ValidateName will validate a file or folder name, returning an error when it is not accepted
func ValidateName(name string, validators []Validator) error {
	for _, v := range validators {
		if err := v(name); err != nil {
			return fmt.Errorf("name validation failed: %w", err)
		}
	}
	return nil
}

func invalidNames(bad []string) Validator {
	return func(s string) error {
		for _, b := range bad {
			if s == b {
				return fmt.Errorf("must not be %s", b)
			}
		}
		return nil
	}
}

func notEmpty() Validator {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New("must not be empty")
		}
		return nil
	}
}

func doesNotContain(bad []string) Validator {
	return func(s string) error {
		for _, b := range bad {
			if strings.Contains(s, b) {
				return fmt.Errorf("must not contain %s", b)
			}
		}
		return nil
	}
}

func isShorterThan(maxLength int) Validator {
	return func(s string) error {
		if len(s) > maxLength {
			return fmt.Errorf("must be shorter than %d", maxLength)
		}
		return nil
	}
}
