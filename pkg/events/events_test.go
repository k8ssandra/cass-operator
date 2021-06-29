package events

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	api "github.com/k8ssandra/cass-operator/api/v1beta1"
	"k8s.io/client-go/tools/record"
)

type MockInfoLogger struct {
	mock.Mock
}

func (l *MockInfoLogger) Info(msg string, keysAndValues ...interface{}) {
	values := append([]interface{}{msg}, keysAndValues...)
	_ = l.Called(values...)
}

func (l *MockInfoLogger) Enabled() bool {
	args := l.Called()
	return args.Bool(0)
}

func (l *MockInfoLogger) Error(err error, msg string, keysAndValues ...interface{}) {
}

func (l *MockInfoLogger) V(level int) logr.InfoLogger {
	return l
}

func (l *MockInfoLogger) WithValues(keysAndValues ...interface{}) logr.InfoLogger {
	return l
}

func (l *MockInfoLogger) WithName(name string) logr.InfoLogger {
	return l
}

func TestLoggingEventRecorder(t *testing.T) {
	logger := &MockInfoLogger{}
	recorder := &LoggingEventRecorder{
		EventRecorder: &record.FakeRecorder{},
		ReqLogger:     logger}
	dc := &api.CassandraDatacenter{}

	logger.On("Info",
		"Some event message",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Times(3)

	recorder.Eventf(dc, "SomeType", "SomeReason", "Some %s message", "event")
	recorder.Event(dc, "SomeType", "SomeReason", "Some event message")
	recorder.AnnotatedEventf(
		dc, map[string]string{}, "SomeType", "SomeReason", "Some %s message", "event")

	logger.AssertExpectations(t)

	// InfoLogger.Info(...) takes key value pairs as var args. The order they
	// appear in the call is not relevant and we do not want to overspecify in
	// this test by requring a certain order. Consequently, we do the below
	// finagling to make sure the right args were passed.

	for j := 0; j < 3; j++ {
		keysAndValues := map[string]string{}
		args := logger.Calls[j].Arguments
		for i := 1; i < len(args)-1; i += 2 {
			key, ok := args[i].(string)
			assert.True(t, ok)
			value, ok := args[i+1].(string)
			assert.True(t, ok)
			keysAndValues[key] = value
		}

		assert.Equal(
			t,
			map[string]string{"eventType": "SomeType", "reason": "SomeReason"},
			keysAndValues)
	}
}
