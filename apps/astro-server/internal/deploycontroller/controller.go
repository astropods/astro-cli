package deploycontroller

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	appslisters "k8s.io/client-go/listers/apps/v1"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const (
	// managedByLabelSelector scopes every informer to objects the applier
	// stamped with app.kubernetes.io/managed-by=astro-server, so the controller
	// only watches our workloads (not the whole cluster).
	managedByLabelSelector = "app.kubernetes.io/managed-by=astro-server"

	// defaultResync re-delivers all cached objects on this interval, forcing a
	// full re-derivation. This is the self-healing backstop that replaced the
	// old periodic reconcile job — it also recovers from any missed event.
	defaultResync = 2 * time.Minute

	// clusterPollInterval is how often we re-poll the registry to discover
	// clusters added at runtime (the registry has no add callback).
	clusterPollInterval = time.Minute

	// syncWorkers is the number of goroutines draining the workqueue.
	syncWorkers = 4
)

// Store is the narrow persistence surface the controller needs (satisfied by
// *deploymentstore.Store), kept minimal for testability.
type Store interface {
	GetLatestDeploymentByNamespace(namespace string) (*deploymentstore.Deployment, error)
	ReplaceWorkloadStatuses(deploymentID string, statuses []deploymentstore.WorkloadStatus) error
}

// queueKey identifies a unit of work: re-derive all workload statuses for one
// deployment namespace on one cluster. Bursty per-object events for the same
// namespace coalesce to a single key in the workqueue.
type queueKey struct {
	cluster   string // registry cluster id; "" = primary
	namespace string
}

// Controller watches managed workloads across clusters and persists their
// observed health. Phase 1: observe + persist only (shadow mode).
type Controller struct {
	registry *k8s.Registry
	store    Store
	log      *logger.Logger
	queue    workqueue.TypedRateLimitingInterface[queueKey]

	mu       sync.Mutex
	watchers map[string]*clusterWatcher // keyed by cluster id
}

// clusterWatcher holds one cluster's informer factory and listers.
type clusterWatcher struct {
	factory   informers.SharedInformerFactory
	deploys   appslisters.DeploymentLister
	statefuls appslisters.StatefulSetLister
	jobs      batchlisters.JobLister
	cronJobs  batchlisters.CronJobLister
	// synced flips true once every informer has completed its initial LIST.
	// sync() refuses to write before this to avoid persisting a partial set
	// from an event that fires while other caches are still warming.
	synced *atomic.Bool
}

// New constructs a controller. registry may be nil, in which case Run is a
// no-op (e.g. local dev without a cluster).
func New(log *logger.Logger, registry *k8s.Registry, store Store) *Controller {
	return &Controller{
		registry: registry,
		store:    store,
		log:      log,
		queue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[queueKey](),
		),
		watchers: map[string]*clusterWatcher{},
	}
}

// Run starts the controller and blocks until ctx is cancelled. It discovers
// clusters, starts a per-cluster informer set, and drains the workqueue with a
// pool of sync workers.
func (c *Controller) Run(ctx context.Context) {
	if c.registry == nil {
		c.log.Info("deploycontroller: no k8s registry, controller disabled")
		return
	}
	c.log.Info("deploycontroller: starting")

	// Discover clusters now and periodically thereafter (add-only).
	c.discoverClusters(ctx)
	go c.clusterDiscoveryLoop(ctx)

	// Drain the workqueue.
	var wg sync.WaitGroup
	for range syncWorkers {
		wg.Go(func() { c.runWorker(ctx) })
	}

	<-ctx.Done()
	c.log.Info("deploycontroller: shutting down")
	c.queue.ShutDown()
	wg.Wait()

	c.mu.Lock()
	for _, w := range c.watchers {
		w.factory.Shutdown()
	}
	c.mu.Unlock()
}

func (c *Controller) clusterDiscoveryLoop(ctx context.Context) {
	t := time.NewTicker(clusterPollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.discoverClusters(ctx)
		}
	}
}

// discoverClusters starts an informer set for any cluster not already watched.
// Removal is not handled here — a deleted cluster's watches fail and idle; the
// registry is the source of truth for reads.
func (c *Controller) discoverClusters(ctx context.Context) {
	entries, err := c.registry.List(ctx, true)
	if err != nil {
		c.log.Warn("deploycontroller: list clusters failed", "error", err)
		return
	}
	for _, e := range entries {
		c.mu.Lock()
		_, exists := c.watchers[e.ID]
		c.mu.Unlock()
		if exists {
			continue
		}
		if err := c.startWatcher(ctx, e.ID, e.IsPrimary); err != nil {
			c.log.Warn("deploycontroller: start watcher failed", "cluster_id", e.ID, "error", err)
		}
	}
}

func (c *Controller) startWatcher(ctx context.Context, clusterID string, isPrimary bool) error {
	kc, err := c.clusterClient(ctx, clusterID, isPrimary)
	if err != nil {
		return fmt.Errorf("resolve cluster client: %w", err)
	}

	factory := informers.NewSharedInformerFactoryWithOptions(
		kc.Clientset(), defaultResync,
		informers.WithTweakListOptions(func(o *metav1.ListOptions) {
			o.LabelSelector = managedByLabelSelector
		}),
	)

	depInf := factory.Apps().V1().Deployments()
	stsInf := factory.Apps().V1().StatefulSets()
	jobInf := factory.Batch().V1().Jobs()
	cronInf := factory.Batch().V1().CronJobs()
	podInf := factory.Core().V1().Pods()

	w := &clusterWatcher{
		factory:   factory,
		deploys:   depInf.Lister(),
		statefuls: stsInf.Lister(),
		jobs:      jobInf.Lister(),
		cronJobs:  cronInf.Lister(),
		synced:    &atomic.Bool{},
	}

	handler := c.eventHandler(clusterID)
	informerList := []cache.SharedIndexInformer{
		depInf.Informer(), stsInf.Informer(), jobInf.Informer(), cronInf.Informer(), podInf.Informer(),
	}
	var syncFuncs []cache.InformerSynced
	for _, inf := range informerList {
		if _, err := inf.AddEventHandler(handler); err != nil {
			return fmt.Errorf("add event handler: %w", err)
		}
		// Surface watch/list failures (cluster unreachable, RBAC) instead of
		// letting client-go retry them silently.
		if err := inf.SetWatchErrorHandler(func(_ *cache.Reflector, e error) {
			c.log.Warn("deploycontroller: watch error", "cluster_id", clusterID, "error", e)
		}); err != nil {
			return fmt.Errorf("set watch error handler: %w", err)
		}
		syncFuncs = append(syncFuncs, inf.HasSynced)
	}

	stopCh := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(stopCh)
	}()
	factory.Start(stopCh)

	// Gate writes until every informer has finished its initial LIST, so a
	// sync triggered by an early event never persists a partial/empty set.
	go func() {
		if cache.WaitForCacheSync(stopCh, syncFuncs...) {
			w.synced.Store(true)
			c.log.Info("deploycontroller: caches synced", "cluster_id", clusterID)
		}
	}()

	c.mu.Lock()
	c.watchers[clusterID] = w
	c.mu.Unlock()

	c.log.Info("deploycontroller: watching cluster", "cluster_id", clusterID, "primary", isPrimary)
	return nil
}

func (c *Controller) clusterClient(ctx context.Context, clusterID string, isPrimary bool) (k8s.ClusterClient, error) {
	if isPrimary || clusterID == "" {
		return c.registry.Default(), nil
	}
	return c.registry.Get(ctx, clusterID)
}

// eventHandler enqueues the (cluster, namespace) key for any object change.
func (c *Controller) eventHandler(clusterID string) cache.ResourceEventHandlerFuncs {
	enqueue := func(obj any) {
		ns, ok := namespaceOf(obj)
		if !ok || ns == "" {
			return
		}
		c.queue.Add(queueKey{cluster: clusterID, namespace: ns})
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    enqueue,
		UpdateFunc: func(_, newObj any) { enqueue(newObj) },
		DeleteFunc: enqueue,
	}
}

func (c *Controller) runWorker(ctx context.Context) {
	for {
		key, shutdown := c.queue.Get()
		if shutdown {
			return
		}
		c.processKey(ctx, key)
	}
}

func (c *Controller) processKey(ctx context.Context, key queueKey) {
	defer c.queue.Done(key)
	// Recover so one malformed object can't permanently kill a sync worker;
	// the panicking key is dropped and the next resync retries it.
	defer func() {
		if r := recover(); r != nil {
			c.log.Error("deploycontroller: recovered panic in sync",
				"cluster_id", key.cluster, "namespace", key.namespace, "panic", r)
		}
	}()
	if err := c.sync(ctx, key); err != nil {
		c.log.Warn("deploycontroller: sync failed, requeueing",
			"cluster_id", key.cluster, "namespace", key.namespace, "error", err)
		c.queue.AddRateLimited(key)
		return
	}
	c.queue.Forget(key)
}

// sync re-derives the full workload-status set for a deployment namespace from
// the informer cache and persists it. Shadow mode: it does not touch the
// deployment-level lifecycle status.
func (c *Controller) sync(_ context.Context, key queueKey) error {
	c.mu.Lock()
	w := c.watchers[key.cluster]
	c.mu.Unlock()
	if w == nil {
		return nil
	}
	if !w.synced.Load() {
		// Caches still warming — don't persist a partial set. Retry shortly.
		c.queue.AddAfter(key, time.Second)
		return nil
	}

	// Resolve including undeployed so a torn-down namespace still yields the
	// deployment id — the listers then return no workloads and we clear the
	// deployment's stale rows below.
	dep, err := c.store.GetLatestDeploymentByNamespace(key.namespace)
	if err != nil {
		return fmt.Errorf("get deployment by namespace: %w", err)
	}
	if dep == nil {
		return nil
	}
	// Only the watcher for the deployment's routing cluster owns its rows.
	// During a cross-cluster migration the same namespace can transiently
	// exist on the old and new cluster; without this guard both would write.
	if dep.EffectiveClusterID() != key.cluster {
		return nil
	}

	var statuses []deploymentstore.WorkloadStatus
	sel := labels.Everything() // informer is already label-scoped

	deploys, err := w.deploys.Deployments(key.namespace).List(sel)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}
	for _, d := range deploys {
		statuses = append(statuses, deriveDeploymentHealth(d))
	}

	statefuls, err := w.statefuls.StatefulSets(key.namespace).List(sel)
	if err != nil {
		return fmt.Errorf("list statefulsets: %w", err)
	}
	for _, s := range statefuls {
		statuses = append(statuses, deriveStatefulSetHealth(s))
	}

	jobs, err := w.jobs.Jobs(key.namespace).List(sel)
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}
	for _, j := range jobs {
		statuses = append(statuses, deriveJobHealth(j))
	}

	cronJobs, err := w.cronJobs.CronJobs(key.namespace).List(sel)
	if err != nil {
		return fmt.Errorf("list cronjobs: %w", err)
	}
	for _, cj := range cronJobs {
		statuses = append(statuses, deriveCronJobHealth(cj))
	}

	if err := c.store.ReplaceWorkloadStatuses(dep.ID, statuses); err != nil {
		return fmt.Errorf("replace workload statuses: %w", err)
	}

	c.log.Debug("deploycontroller: synced workload statuses",
		"deployment_id", dep.ID, "namespace", key.namespace, "workloads", len(statuses))
	return nil
}

// namespaceOf extracts the namespace from an informer object, unwrapping the
// tombstone form delivered on some deletes.
func namespaceOf(obj any) (string, bool) {
	if tomb, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tomb.Obj
	}
	m, err := meta.Accessor(obj)
	if err != nil {
		return "", false
	}
	return m.GetNamespace(), true
}
