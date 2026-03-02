package kv

import "errors"

var ErrNotFound = errors.New("not found")
var ErrTooLarge = errors.New("item is too large")
