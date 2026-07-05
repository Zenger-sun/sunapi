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

func errBadRequest(msg string) error {
	if msg == "" {
		return errors.New("bad request")
	}
	return errors.New(msg)
}

func errUnauthorized(msg string) error {
	if msg == "" {
		return errors.New("unauthorized")
	}
	return errors.New(msg)
}
