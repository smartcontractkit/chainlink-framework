package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	NodeBalance = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "node_balance", Help: "Account balances"},
		[]string{"account", "chainID", "chainFamily"},
	)
)
