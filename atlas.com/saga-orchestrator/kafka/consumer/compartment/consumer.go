package compartment

import (
	consumer2 "atlas-saga-orchestrator/kafka/consumer"
	"atlas-saga-orchestrator/kafka/message/compartment"
	"atlas-saga-orchestrator/saga"
	"context"
	"github.com/Chronicle20/atlas-kafka/consumer"
	"github.com/Chronicle20/atlas-kafka/handler"
	"github.com/Chronicle20/atlas-kafka/message"
	"github.com/Chronicle20/atlas-kafka/topic"
	"github.com/Chronicle20/atlas-model/model"
	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
)

func InitConsumers(l logrus.FieldLogger) func(func(config consumer.Config, decorators ...model.Decorator[consumer.Config])) func(consumerGroupId string) {
	return func(rf func(config consumer.Config, decorators ...model.Decorator[consumer.Config])) func(consumerGroupId string) {
		return func(consumerGroupId string) {
			rf(consumer2.NewConfig(l)("compartment_status_event")(compartment.EnvEventTopicStatus)(consumerGroupId), consumer.SetHeaderParsers(consumer.SpanHeaderParser, consumer.TenantHeaderParser), consumer.SetStartOffset(kafka.LastOffset))
		}
	}
}

func InitHandlers(l logrus.FieldLogger) func(rf func(topic string, handler handler.Handler) (string, error)) {
	return func(rf func(topic string, handler handler.Handler) (string, error)) {
		var t string
		t, _ = topic.EnvProvider(l)(compartment.EnvEventTopicStatus)()
		_, _ = rf(t, message.AdaptHandler(message.PersistentConfig(handleCompartmentCreatedEvent)))
		_, _ = rf(t, message.AdaptHandler(message.PersistentConfig(handleCompartmentCreationFailedEvent)))
		_, _ = rf(t, message.AdaptHandler(message.PersistentConfig(handleCompartmentDeletedEvent)))
		_, _ = rf(t, message.AdaptHandler(message.PersistentConfig(handleCompartmentErrorEvent)))
	}
}

func handleCompartmentCreatedEvent(l logrus.FieldLogger, ctx context.Context, e compartment.StatusEvent[compartment.CreatedStatusEventBody]) {
	if e.Type != compartment.StatusEventTypeCreated {
		return
	}

	// Complete the current step for regular compartment creation
	_ = saga.NewProcessor(l, ctx).StepCompleted(e.TransactionId, true)
}

func handleCompartmentCreationFailedEvent(l logrus.FieldLogger, ctx context.Context, e compartment.StatusEvent[compartment.CreationFailedStatusEventBody]) {
	if e.Type != compartment.StatusEventTypeCreationFailed {
		return
	}

	l.WithFields(logrus.Fields{
		"transaction_id": e.TransactionId.String(),
		"character_id":   e.CharacterId,
		"error_code":     e.Body.ErrorCode,
		"error_message":  e.Body.Message,
	}).Error("Asset creation failed, marking saga step as failed")

	// Mark the saga step as failed
	sagaProcessor := saga.NewProcessor(l, ctx)
	_ = sagaProcessor.StepCompleted(e.TransactionId, false)
}

func handleCompartmentDeletedEvent(l logrus.FieldLogger, ctx context.Context, e compartment.StatusEvent[compartment.DeletedStatusEventBody]) {
	if e.Type != compartment.StatusEventTypeDeleted {
		return
	}
	_ = saga.NewProcessor(l, ctx).StepCompleted(e.TransactionId, true)
}

func handleCompartmentErrorEvent(l logrus.FieldLogger, ctx context.Context, e compartment.StatusEvent[compartment.ErrorEventBody]) {
	if e.Type != compartment.StatusEventTypeError {
		return
	}

	l.WithFields(logrus.Fields{
		"transaction_id": e.TransactionId.String(),
		"error_code":     e.Body.ErrorCode,
		"character_id":   e.CharacterId,
	}).Error("Compartment operation failed")

	_ = saga.NewProcessor(l, ctx).StepCompleted(e.TransactionId, false)
}
