package policystore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/zpol/katana/internal/policy"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

var imagePolicyGVR = schema.GroupVersionResource{
	Group: crdGroup, Version: crdVersion, Resource: crdResource,
}

// CRDStore watches ImagePolicy CRDs and serves policy CRUD via the Kubernetes API.
type CRDStore struct {
	dyn      dynamic.Interface
	informer cache.SharedIndexInformer
	stopCh   chan struct{}
	mu       sync.RWMutex
	ready    bool
}

// NewCRD connects in-cluster (or KUBECONFIG) and starts an informer.
func NewCRD(ctx context.Context) (*CRDStore, error) {
	cfg, err := restConfig()
	if err != nil {
		return nil, fmt.Errorf("kubernetes config: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dyn, 5*time.Minute, metav1.NamespaceAll, nil)
	inf := factory.ForResource(imagePolicyGVR).Informer()
	s := &CRDStore{
		dyn:      dyn,
		informer: inf,
		stopCh:   make(chan struct{}),
	}
	inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { s.setReady(true) },
		UpdateFunc: func(_, _ interface{}) { s.setReady(true) },
		DeleteFunc: func(_ interface{}) { s.setReady(true) },
	})
	factory.Start(s.stopCh)
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		return nil, fmt.Errorf("imagepolicy informer sync timeout")
	}
	s.setReady(true)
	return s, nil
}

func restConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
}

func (s *CRDStore) setReady(v bool) {
	s.mu.Lock()
	s.ready = v
	s.mu.Unlock()
}

func (s *CRDStore) Ready(_ context.Context) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready && s.informer.HasSynced()
}

func (s *CRDStore) List(_ context.Context) ([]policy.Policy, error) {
	return policiesFromCache(s.informer.GetStore().List())
}

func policiesFromCache(items []interface{}) ([]policy.Policy, error) {
	out := make([]policy.Policy, 0, len(items))
	for _, raw := range items {
		obj, ok := raw.(*unstructured.Unstructured)
		if !ok {
			return nil, fmt.Errorf("imagepolicy cache: unexpected type %T", raw)
		}
		p, err := unstructuredToPolicy(obj)
		if err != nil {
			name := obj.GetName()
			return nil, fmt.Errorf("imagepolicy %q: %w", name, err)
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *CRDStore) Get(_ context.Context, id string) (policy.Policy, error) {
	raw, exists, err := s.informer.GetStore().GetByKey(id)
	if err != nil {
		return policy.Policy{}, err
	}
	if !exists {
		return policy.Policy{}, ErrNotFound
	}
	obj, ok := raw.(*unstructured.Unstructured)
	if !ok {
		return policy.Policy{}, fmt.Errorf("unexpected cache type")
	}
	return unstructuredToPolicy(obj)
}

func (s *CRDStore) Create(ctx context.Context, p policy.Policy) (policy.Policy, error) {
	obj := policyToUnstructured(p)
	created, err := s.dyn.Resource(imagePolicyGVR).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return policy.Policy{}, err
	}
	return unstructuredToPolicy(created)
}

func (s *CRDStore) Update(ctx context.Context, p policy.Policy) (policy.Policy, error) {
	existing, err := s.dyn.Resource(imagePolicyGVR).Get(ctx, p.ID, metav1.GetOptions{})
	if err != nil {
		return policy.Policy{}, err
	}
	next := policyToUnstructured(p)
	next.SetResourceVersion(existing.GetResourceVersion())
	updated, err := s.dyn.Resource(imagePolicyGVR).Update(ctx, next, metav1.UpdateOptions{})
	if err != nil {
		return policy.Policy{}, err
	}
	return unstructuredToPolicy(updated)
}

func (s *CRDStore) Patch(ctx context.Context, id string, patch map[string]json.RawMessage) (policy.Policy, error) {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return policy.Policy{}, err
	}
	return s.Update(ctx, applyPatchToPolicy(cur, patch))
}

func (s *CRDStore) Delete(ctx context.Context, id string) error {
	err := s.dyn.Resource(imagePolicyGVR).Delete(ctx, id, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

func (s *CRDStore) Close() error {
	close(s.stopCh)
	return nil
}
