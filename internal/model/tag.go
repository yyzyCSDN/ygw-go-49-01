package model

// Tag binds a mutable name inside a repository to one manifest digest.
type Tag struct {
	Repo   string
	Name   string
	Digest string
}
