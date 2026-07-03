package sunapi

import "errors"

func errNotFound(subject string) error {
	if subject == "" {
		return errors.New("not found")
	}
	return errors.New(subject + " not found")
}

func errConflict(msg string) error {
	if msg == "" {
		return errors.New("conflict")
	}
	return errors.New(msg)
}
