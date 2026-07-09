package automation

import "context"

const (
	automationEventStarted         = "automation.job.started"
	automationEventFinished        = "automation.job.finished"
	automationEventFailed          = "automation.job.failed"
	automationEventSkipped         = "automation.job.skipped"
	automationEventSvcStarted      = "automation.service.started"
	automationEventSvcStopped      = "automation.service.stopped"
	automationEventDeliveryStarted = "automation.delivery.started"
	automationEventDeliveryDone    = "automation.delivery.finished"
	automationEventBackoff         = "automation.failure.backoff"
)

type Logger interface {
	Debug(string, map[string]any)
	Info(string, map[string]any)
	Warn(string, map[string]any)
	Error(string, map[string]any)
}

type Tracer interface {
	Record(context.Context, string, any)
}

func (s *Service) record(ctx context.Context, level string, message string, event string, fields map[string]any) {
	if fields == nil {
		fields = make(map[string]any)
	}
	if s != nil && s.logger != nil {
		switch level {
		case "debug":
			s.logger.Debug(message, fields)
		case "warn":
			s.logger.Warn(message, fields)
		case "error":
			s.logger.Error(message, fields)
		default:
			s.logger.Info(message, fields)
		}
	}
	if s != nil && s.tracer != nil {
		s.tracer.Record(ctx, event, fields)
	}
}
