package metrics

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Runtime is cluster/process identity collected on each Prometheus scrape.
type Runtime struct {
	Version      string
	PolicySource string
	DryRun       bool
	Ready        func() bool
	JFrogUp      func() bool
}

type runtimeCollector struct {
	rt     Runtime
	kube   kubernetes.Interface
	info   *prometheus.Desc
	ready  *prometheus.Desc
	jfrog  *prometheus.Desc
	nodes  *prometheus.Desc
	k8sVer *prometheus.Desc
}

// NewRuntimeCollector builds a Prometheus collector for info + kube inventory.
func NewRuntimeCollector(rt Runtime) prometheus.Collector {
	c := &runtimeCollector{
		rt: rt,
		info: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "info"),
			"Katana process identity (always 1).",
			[]string{"cluster", "version", "policy_source", "dry_run"},
			nil,
		),
		ready: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "policies_ready"),
			"1 if the policy store is ready (CRD informer synced or sqlite).",
			[]string{"cluster"},
			nil,
		),
		jfrog: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "jfrog_up"),
			"1 if JFrog Xray is configured and last status was reachable.",
			[]string{"cluster"},
			nil,
		),
		nodes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cluster_nodes"),
			"Node count by readiness.",
			[]string{"cluster", "status"},
			nil,
		),
		k8sVer: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cluster_info"),
			"Kubernetes apiserver version (always 1).",
			[]string{"cluster", "k8s_version"},
			nil,
		),
	}
	c.kube = inClusterClient()
	return c
}

func inClusterClient() kubernetes.Interface {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := strings.TrimSpace(os.Getenv("KUBECONFIG"))
		if kubeconfig == "" {
			return nil
		}
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil
		}
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil
	}
	return cs
}

func (c *runtimeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.info
	ch <- c.ready
	ch <- c.jfrog
	ch <- c.nodes
	ch <- c.k8sVer
}

func (c *runtimeCollector) Collect(ch chan<- prometheus.Metric) {
	cl := ClusterName()
	dry := "false"
	if c.rt.DryRun {
		dry = "true"
	}
	src := c.rt.PolicySource
	if src == "" {
		src = "unknown"
	}
	ver := c.rt.Version
	if ver == "" {
		ver = "dev"
	}
	ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1, cl, ver, src, dry)

	ready := 0.0
	if c.rt.Ready != nil && c.rt.Ready() {
		ready = 1
	}
	ch <- prometheus.MustNewConstMetric(c.ready, prometheus.GaugeValue, ready, cl)

	jf := 0.0
	if c.rt.JFrogUp != nil && c.rt.JFrogUp() {
		jf = 1
	}
	ch <- prometheus.MustNewConstMetric(c.jfrog, prometheus.GaugeValue, jf, cl)

	readyN, notReadyN, k8s := c.inventory()
	ch <- prometheus.MustNewConstMetric(c.nodes, prometheus.GaugeValue, float64(readyN), cl, "ready")
	ch <- prometheus.MustNewConstMetric(c.nodes, prometheus.GaugeValue, float64(notReadyN), cl, "not_ready")
	if k8s == "" {
		k8s = "unknown"
	}
	ch <- prometheus.MustNewConstMetric(c.k8sVer, prometheus.GaugeValue, 1, cl, k8s)
}

func (c *runtimeCollector) inventory() (ready, notReady int, k8sVersion string) {
	if c.kube == nil {
		return 0, 0, "unknown"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if info, err := c.kube.Discovery().ServerVersion(); err == nil && info != nil {
		k8sVersion = strings.TrimPrefix(info.GitVersion, "v")
		if k8sVersion == "" {
			k8sVersion = info.String()
		}
	} else {
		incCollectorError()
		k8sVersion = "unknown"
	}
	list, err := c.kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		incCollectorError()
		return 0, 0, k8sVersion
	}
	for i := range list.Items {
		n := &list.Items[i]
		ok := false
		for _, cond := range n.Status.Conditions {
			if strings.EqualFold(string(cond.Type), "Ready") {
				ok = strings.EqualFold(string(cond.Status), "True")
				break
			}
		}
		if ok {
			ready++
		} else {
			notReady++
		}
	}
	return ready, notReady, k8sVersion
}
