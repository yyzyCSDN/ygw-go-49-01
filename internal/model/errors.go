package model

import "errors"

var (
	// ErrNotFound is returned when a requested repository, blob, manifest,
	// tag or upload session does not exist.
	ErrNotFound = errors.New("artifactregistry: not found")
	// ErrAlreadyExists is returned when creating an entity that already exists.
	ErrAlreadyExists = errors.New("artifactregistry: already exists")
	// ErrInvalidArgument is returned when an argument fails basic validation.
	ErrInvalidArgument = errors.New("artifactregistry: invalid argument")
	// ErrInvalidState is returned when a state machine rejects a transition.
	ErrInvalidState = errors.New("artifactregistry: invalid state transition")
	// ErrUnfinished is returned when an operation depends on an upload that has
	// not reached the committed state.
	ErrUnfinished = errors.New("artifactregistry: blob upload not finished")
	// ErrConflict is returned when a concurrent update invalidated the request.
	ErrConflict = errors.New("artifactregistry: conflict")
	// ErrUnauthorized is returned when a token is missing, expired or not bound
	// to the session it is used with.
	ErrUnauthorized = errors.New("artifactregistry: unauthorized")
	// ErrCorrupt is returned when content does not match its recorded digest.
	ErrCorrupt = errors.New("artifactregistry: digest mismatch")
	// ErrVerificationPending is returned when an artifact is not yet verified.
	ErrVerificationPending = errors.New("artifactregistry: verification pending")
)
