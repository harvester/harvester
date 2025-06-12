package metrics

import (
	"context"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"k8s.io/apimachinery/pkg/labels"

	ctlclusterv1 "github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io/v1beta1"

	"github.com/prometheus/client_golang/prometheus"
	//"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
)

const (
	nodeMachineMetricsControllerName = "node-machine-metrics-controller"
)

type nodeMachineMetricsHandler struct {
	nodes        ctlcorev1.NodeController
	nodeCache    ctlcorev1.NodeCache
	machines     ctlclusterv1.MachineController
	machineCache ctlclusterv1.MachineCache

	metrics *metrics
}

type metrics struct {
	node_count             prometheus.Gauge
	deleting_node_count    prometheus.Gauge
	machine_count          prometheus.Gauge
	deleting_machine_count prometheus.Gauge
}

func NodeMachineMetricsRegister(ctx context.Context, management *config.Management, _ config.Options) error {
	logrus.Infof("node machine metrics is registered")
	nodes := management.CoreFactory.Core().V1().Node()
	machines := management.ClusterFactory.Cluster().V1beta1().Machine()

	nodeMachineMetricsHandler := &nodeMachineMetricsHandler{

		nodes:        nodes,
		nodeCache:    nodes.Cache(),
		machines:     machines,
		machineCache: machines.Cache(),
	}

	nodeMachineMetricsHandler.metrics = initMetrics(management)

	nodes.OnChange(ctx, nodeMachineMetricsControllerName, nodeMachineMetricsHandler.OnNodeChanged)
	machines.OnChange(ctx, nodeMachineMetricsControllerName, nodeMachineMetricsHandler.OnMachineChanged)
	return nil
}

func initMetrics(management *config.Management) *metrics {
	m := &metrics{
		node_count: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "harvester_node_count",
			Help: "Current nodes on the Harvester cluster.",
		}),
		deleting_node_count: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "harvester_deleting_node_count",
			Help: "Current being-deleted nodes on the Harvester cluster.",
		}),
		machine_count: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "harvester_machine_count",
			Help: "Current machines on the Harvester cluster.",
		}),
		deleting_machine_count: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "harvester_deleting_machine_count",
			Help: "Current being-deleted machines on the Harvester cluster.",
		}),
	}

	management.GetMetricsRegistry().MustRegister(m.node_count)
	management.GetMetricsRegistry().MustRegister(m.deleting_node_count)
	management.GetMetricsRegistry().MustRegister(m.machine_count)
	management.GetMetricsRegistry().MustRegister(m.deleting_machine_count)
	return m
}

func (n *nodeMachineMetricsHandler) OnNodeChanged(_ string, node *corev1.Node) (*corev1.Node, error) {
	nodes, err := n.nodeCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	n.metrics.node_count.Set(float64(len(nodes)))

	cnt := 0
	for _, n := range nodes {
		if n.DeletionTimestamp != nil {
			cnt += 1
		}
	}

	n.metrics.deleting_node_count.Set(float64(cnt))

	return node, nil
}

func (n *nodeMachineMetricsHandler) OnMachineChanged(_ string, machine *clusterv1.Machine) (*clusterv1.Machine, error) {
	machines, err := n.machineCache.List(util.FleetLocalNamespaceName, labels.Everything())
	if err != nil {
		return nil, err
	}

	n.metrics.machine_count.Set(float64(len(machines)))

	cnt := 0
	for _, m := range machines {
		if m.DeletionTimestamp != nil {
			cnt += 1
		}
	}

	n.metrics.deleting_machine_count.Set(float64(cnt))

	return machine, nil
}
