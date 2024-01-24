/*
Package client provides a client that can be configured to point to a kubernetes cluster's kube-api and creates requests
for a specified kubernetes resource. Package client also contains functions for a sharedclientfactory which manages the
multiple clients needed to interact with multiple kubernetes resource types.
*/
package client

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

// Client performs CRUD like operations on a specific GVR.
type Client struct {
	// Default RESTClient
	RESTClient rest.Interface
	// Config that can be used to build a RESTClient with custom options
	Config     rest.Config
	timeout    time.Duration
	Namespaced bool
	GVR        schema.GroupVersionResource
	resource   string
	prefix     []string
	apiVersion string
	kind       string
}

// IsNamespaced determines if the give GroupVersionResource is namespaced using the given RESTMapper.
// returns true if namespaced and an error if the scope could not be determined.
func IsNamespaced(gvr schema.GroupVersionResource, mapper meta.RESTMapper) (bool, error) {
	kind, err := mapper.KindFor(gvr)
	if err != nil {
		return false, err
	}

	mapping, err := mapper.RESTMapping(kind.GroupKind(), kind.Version)
	if err != nil {
		return false, err
	}

	return mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

// WithAgent attempts to return a copy of the Client but
// with a new restClient created with the passed in userAgent.
func (c *Client) WithAgent(userAgent string) (*Client, error) {
	client := *c
	config := c.Config
	config.UserAgent = userAgent
	restClient, err := rest.UnversionedRESTClientFor(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to created restClient with userAgent [%s]: %w", userAgent, err)
	}
	client.RESTClient = restClient
	client.Config = config
	return &client, nil
}

// WithImpersonation attempts to return a copy of the Client but
// with a new restClient created with the passed in impersonation configuration.
func (c *Client) WithImpersonation(impersonate rest.ImpersonationConfig) (*Client, error) {
	client := *c
	config := c.Config
	config.Impersonate = impersonate
	restClient, err := rest.UnversionedRESTClientFor(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to created restClient with impersonation [%v]: %w", impersonate, err)
	}
	client.RESTClient = restClient
	client.Config = config
	return &client, nil
}

// NewClient will create a client for the given GroupResourceVersion and Kind.
// If namespaced is set to true all request will be sent with the scoped to a namespace.
// The namespaced option can be changed after creation with the client.Namespace variable.
// defaultTimeout will be used to set the timeout for all request from this client. The value of 0 is used to specify an infinite timeout.
// request will return if the provided context is canceled regardless of the value of defaultTimeout.
// Changing the value of client.GVR after it's creation of NewClient will not affect future request.
func NewClient(gvr schema.GroupVersionResource, kind string, namespaced bool, client rest.Interface, defaultTimeout time.Duration) *Client {
	var (
		prefix []string
	)

	if gvr.Group == "" {
		prefix = []string{
			"api",
			gvr.Version,
		}
	} else {
		prefix = []string{
			"apis",
			gvr.Group,
			gvr.Version,
		}
	}

	c := &Client{
		RESTClient: client,
		timeout:    defaultTimeout,
		Namespaced: namespaced,
		GVR:        gvr,
		prefix:     prefix,
		resource:   gvr.Resource,
	}
	c.apiVersion, c.kind = gvr.GroupVersion().WithKind(kind).ToAPIVersionAndKind()
	return c
}

func noop() {}

// setupCtx wraps the provided context with client.timeout, and returns the new context and it's cancel func.
// If client.timeout is 0 then the provided context is returned with a noop function instead of a cancel function.
func (c *Client) setupCtx(ctx context.Context) (context.Context, func()) {
	if c.timeout == 0 {
		return ctx, noop
	}

	return context.WithTimeout(ctx, c.timeout)
}

// Get will attempt to find the requested resource with the given name in the given namespace (if client.Namespaced is set to true).
// Get will then attempt to unmarshal the response into the provide result object.
// If the returned response object is of type Status and has .Status != StatusSuccess, the
// additional information in Status will be used to enrich the error.
func (c *Client) Get(ctx context.Context, namespace, name string, result runtime.Object, options metav1.GetOptions) (err error) {
	defer c.setKind(result)
	ctx, cancel := c.setupCtx(ctx)
	defer cancel()
	err = c.RESTClient.Get().
		Prefix(c.prefix...).
		NamespaceIfScoped(namespace, c.Namespaced).
		Resource(c.resource).
		Name(name).
		VersionedParams(&options, metav1.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List will attempt to find resources in the given namespace (if client.Namespaced is set to true).
// List will then attempt to unmarshal the response into the provide result object.
// If the returned response object is of type Status and has .Status != StatusSuccess, the
// additional information in Status will be used to enrich the error.
func (c *Client) List(ctx context.Context, namespace string, result runtime.Object, opts metav1.ListOptions) (err error) {
	ctx, cancel := c.setupCtx(ctx)
	defer cancel()
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	r := c.RESTClient.Get()
	if namespace != "" {
		r = r.NamespaceIfScoped(namespace, c.Namespaced)
	}
	err = r.Resource(c.resource).
		Prefix(c.prefix...).
		VersionedParams(&opts, metav1.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch will attempt to start a watch request with the kube-apiserver for resources in the given namespace (if client.Namespaced is set to true).
// Results will be streamed too the returned watch.Interface.
// The returned watch.Interface is determine by *("k8s.io/client-go/rest").Request.Watch
func (c *Client) Watch(ctx context.Context, namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.injectKind(c.RESTClient.Get().
		Prefix(c.prefix...).
		NamespaceIfScoped(namespace, c.Namespaced).
		Resource(c.resource).
		VersionedParams(&opts, metav1.ParameterCodec).
		Timeout(timeout).
		Watch(ctx))
}

// Create will attempt create the provided object in the given namespace (if client.Namespaced is set to true).
// Create will then attempt to unmarshal the created object from the response into the provide result object.
// If the returned response object is of type Status and has .Status != StatusSuccess, the
// additional information in Status will be used to enrich the error.
func (c *Client) Create(ctx context.Context, namespace string, obj, result runtime.Object, opts metav1.CreateOptions) (err error) {
	defer c.setKind(result)
	ctx, cancel := c.setupCtx(ctx)
	defer cancel()
	err = c.RESTClient.Post().
		Prefix(c.prefix...).
		NamespaceIfScoped(namespace, c.Namespaced).
		Resource(c.resource).
		VersionedParams(&opts, metav1.ParameterCodec).
		Body(obj).
		Do(ctx).
		Into(result)
	return
}

// Update will attempt update the provided object in the given namespace (if client.Namespaced is set to true).
// Update will then attempt to unmarshal the updated object from the response into the provide result object.
// If the returned response object is of type Status and has .Status != StatusSuccess, the
// additional information in Status will be used to enrich the error.
func (c *Client) Update(ctx context.Context, namespace string, obj, result runtime.Object, opts metav1.UpdateOptions) (err error) {
	defer c.setKind(result)
	ctx, cancel := c.setupCtx(ctx)
	defer cancel()
	m, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	err = c.RESTClient.Put().
		Prefix(c.prefix...).
		NamespaceIfScoped(namespace, c.Namespaced).
		Resource(c.resource).
		Name(m.GetName()).
		VersionedParams(&opts, metav1.ParameterCodec).
		Body(obj).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus will attempt update the status on the provided object in the given namespace (if client.Namespaced is set to true).
// UpdateStatus will then attempt to unmarshal the updated object from the response into the provide result object.
// If the returned response object is of type Status and has .Status != StatusSuccess, the
// additional information in Status will be used to enrich the error.
func (c *Client) UpdateStatus(ctx context.Context, namespace string, obj, result runtime.Object, opts metav1.UpdateOptions) (err error) {
	defer c.setKind(result)
	ctx, cancel := c.setupCtx(ctx)
	defer cancel()
	m, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	err = c.RESTClient.Put().
		Prefix(c.prefix...).
		NamespaceIfScoped(namespace, c.Namespaced).
		Resource(c.resource).
		Name(m.GetName()).
		SubResource("status").
		VersionedParams(&opts, metav1.ParameterCodec).
		Body(obj).
		Do(ctx).
		Into(result)
	return
}

// Delete will attempt to delete the resource with the matching name in the given namespace (if client.Namespaced is set to true).
func (c *Client) Delete(ctx context.Context, namespace, name string, opts metav1.DeleteOptions) error {
	ctx, cancel := c.setupCtx(ctx)
	defer cancel()
	return c.RESTClient.Delete().
		Prefix(c.prefix...).
		NamespaceIfScoped(namespace, c.Namespaced).
		Resource(c.resource).
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection will attempt to delete all resource the given namespace (if client.Namespaced is set to true).
func (c *Client) DeleteCollection(ctx context.Context, namespace string, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	ctx, cancel := c.setupCtx(ctx)
	defer cancel()
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.RESTClient.Delete().
		Prefix(c.prefix...).
		NamespaceIfScoped(namespace, c.Namespaced).
		Resource(c.resource).
		VersionedParams(&listOpts, metav1.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch attempts to patch the existing resource with the provided data and patchType that matches the given name in the given namespace (if client.Namespaced is set to true).
// Patch will then attempt to unmarshal the updated object from the response into the provide result object.
// If the returned response object is of type Status and has .Status != StatusSuccess, the
// additional information in Status will be used to enrich the error.
func (c *Client) Patch(ctx context.Context, namespace, name string, pt types.PatchType, data []byte, result runtime.Object, opts metav1.PatchOptions, subresources ...string) (err error) {
	defer c.setKind(result)
	ctx, cancel := c.setupCtx(ctx)
	defer cancel()
	err = c.RESTClient.Patch(pt).
		Prefix(c.prefix...).
		Namespace(namespace).
		Resource(c.resource).
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, metav1.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}

func (c *Client) setKind(obj runtime.Object) {
	if c.kind == "" {
		return
	}
	if _, ok := obj.(*metav1.Status); !ok {
		if meta, err := meta.TypeAccessor(obj); err == nil {
			meta.SetKind(c.kind)
			meta.SetAPIVersion(c.apiVersion)
		}
	}
}

func (c *Client) injectKind(w watch.Interface, err error) (watch.Interface, error) {
	if c.kind == "" || err != nil {
		return w, err
	}

	eventChan := make(chan watch.Event)

	go func() {
		defer close(eventChan)
		for event := range w.ResultChan() {
			c.setKind(event.Object)
			eventChan <- event
		}
	}()

	return &watcher{
		Interface: w,
		eventChan: eventChan,
	}, nil
}

type watcher struct {
	watch.Interface
	eventChan chan watch.Event
}

// ResultChan returns a receive only channel of watch events.
func (w *watcher) ResultChan() <-chan watch.Event {
	return w.eventChan
}
