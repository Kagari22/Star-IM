package mq

import "context"

type NoopPublisher struct{}

func (NoopPublisher) PublishMessageCreated(context.Context, MessageCreatedEvent) error { return nil }
func (NoopPublisher) Close() error                                                     { return nil }
