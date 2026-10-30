package transaction

import "errors"

var (
	ErrTransactionNotFound       = errors.New("transaction not found")
	ErrTransactionAlreadyExists  = errors.New("transaction already exists")
	ErrInvalidStatusTransition   = errors.New("invalid transaction status transition")
	ErrBatchOperationKeyRequired = errors.New("batch operation/idempotency key is required")
	ErrBatchValidation           = errors.New("batch validation failed")
	ErrBatchIdempotencyConflict  = errors.New("batch idempotency key was already used with different data")
	ErrConcurrentModification    = errors.New("transaction was modified concurrently")
	ErrInvalidTransactionValue   = errors.New("invalid transaction value")
)
