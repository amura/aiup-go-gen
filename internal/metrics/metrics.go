package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    AgentMessagesTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "agent_messages_total",
            Help: "Total number of messages processed by agent",
        },
        []string{"agent"},
    )
    ToolCallsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "tool_calls_total",
            Help: "Total number of tool calls",
        },
        []string{"tool", "agent"},
    )
    ToolErrorsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "tool_errors_total",
            Help: "Total number of tool call errors",
        },
        []string{"tool", "agent"},
    )
    ToolLatencySeconds = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "tool_latency_seconds",
            Help:    "Latency of tool calls in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"tool", "agent"},
    )
    OpenAITokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "openai_tokens_total",
			Help: "Total number of tokens sent/received from OpenAI",
		},
		[]string{"type"}, // type: prompt, completion, total
	)
)

func StartMetricsServer(addr string) {
    http.Handle("/metrics", promhttp.Handler())
    go http.ListenAndServe(addr, nil)
}