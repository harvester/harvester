/*
Package tablelistconvert provides a client that will use a table client but convert *UnstructuredList and *Unstructured objects
returned by ByID and List to resemble those returned by non-table clients while preserving some table-related data.
*/
package tablelistconvert

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/data"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sWatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

type Client struct {
	dynamic.ResourceInterface
}

var _ dynamic.ResourceInterface = (*Client)(nil)

type tableConvertWatch struct {
	done   chan struct{}
	events chan k8sWatch.Event
	k8sWatch.Interface
}

// List will return an *UnstructuredList that contains Items instead of just using the Object field to store a table as
// Table Clients do. The items will preserve values for columns in the form of metadata.fields.
func (c *Client) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	list, err := c.ResourceInterface.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	tableToList(list)
	return list, nil
}

func (c *Client) Watch(ctx context.Context, opts metav1.ListOptions) (k8sWatch.Interface, error) {
	w, err := c.ResourceInterface.Watch(ctx, opts)
	if err != nil {
		return nil, err
	}
	events := make(chan k8sWatch.Event)
	done := make(chan struct{})
	eventWatch := &tableConvertWatch{done: done, events: events, Interface: w}
	eventWatch.feed()
	return eventWatch, nil
}

func (w *tableConvertWatch) feed() {
	tableEvents := w.Interface.ResultChan()
	go func() {
		for {
			select {
			case e, ok := <-tableEvents:
				if !ok {
					close(w.events)
					return
				}
				if unstr, ok := e.Object.(*unstructured.Unstructured); ok {
					rowToObject(unstr)
					w.events <- e
				}
			case <-w.done:
				close(w.events)
				return
			}
		}
	}()
}

func (w *tableConvertWatch) ResultChan() <-chan k8sWatch.Event {
	return w.events
}

func (w *tableConvertWatch) Stop() {
	close(w.done)
	w.Interface.Stop()
}

func rowToObject(obj *unstructured.Unstructured) {
	if obj == nil {
		return
	}
	if obj.Object["kind"] != "Table" ||
		(obj.Object["apiVersion"] != "meta.k8s.io/v1" &&
			obj.Object["apiVersion"] != "meta.k8s.io/v1beta1") {
		return
	}

	items := tableToObjects(obj.Object)
	if len(items) == 1 {
		obj.Object = items[0].Object
	}
}

func tableToList(obj *unstructured.UnstructuredList) {
	if obj.Object["kind"] != "Table" ||
		(obj.Object["apiVersion"] != "meta.k8s.io/v1" &&
			obj.Object["apiVersion"] != "meta.k8s.io/v1beta1") {
		return
	}

	obj.Items = tableToObjects(obj.Object)
}

func tableToObjects(obj map[string]interface{}) []unstructured.Unstructured {
	var result []unstructured.Unstructured

	rows, _ := obj["rows"].([]interface{})
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		cells := m["cells"]
		object, ok := m["object"].(map[string]interface{})
		if !ok {
			continue
		}

		data.PutValue(object, cells, "metadata", "fields")
		result = append(result, unstructured.Unstructured{
			Object: object,
		})
	}

	return result
}
