// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/client-go/kubernetes/typed/apps/v1"

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// DaemonSet extends the k8s daemonset.
type DaemonSet struct {
	client v1.DaemonSetInterface
	appsv1.DaemonSet
}

// NewDaemonSet creates a new daemonSet resource.
func NewDaemonSet(client v1.DaemonSetInterface, namespace string, name string, in *appsv1.DaemonSet) *DaemonSet {
	if in == nil {
		in = &appsv1.DaemonSet{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &DaemonSet{client, *in}
}

func (ds *DaemonSet) DeleteOrphan(ctx context.Context) error {
	return nil
}

// Clone returns a copy of the resource.
func (ds *DaemonSet) Clone() Resource {
	clone := *ds
	return &clone
}

// ID returns a comparable ID for the Resource.
func (ds *DaemonSet) ID() ID {
	return ID{"DaemonSet", ds.Name, ds.Namespace}
}

// Apply patches the resource change.
func (ds *DaemonSet) Apply(ctx context.Context) error {
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &ds.DaemonSet)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := ds.client.Patch(ctx, ds.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = ds.client.Create(ctx, &ds.DaemonSet, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "daemon set %q", ds.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	ds.DaemonSet = *res
	return nil
}

// Get refreshes the resource.
func (ds *DaemonSet) Get(ctx context.Context) error {
	res, err := ds.client.Get(ctx, ds.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	ds.DaemonSet = *res
	return nil
}

// Delete removes the resource.
func (ds *DaemonSet) Delete(ctx context.Context) error {
	err := ds.client.Delete(ctx, ds.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s daemon set for deletion")
	}
	return errors.Trace(err)
}

// ComputeStatus returns a juju status for the resource.
func (ds *DaemonSet) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if ds.DeletionTimestamp != nil {
		return "", status.Terminated, ds.DeletionTimestamp.Time, nil
	}
	if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
		return "", status.Active, now, nil
	}
	return "", status.Waiting, now, nil
}

// ListDaemonSets returns a list of daemon sets.
func ListDaemonSets(ctx context.Context, client v1.DaemonSetInterface, namespace string, opts metav1.ListOptions) ([]DaemonSet, error) {
	var items []DaemonSet
	for {
		res, err := client.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, *NewDaemonSet(client, namespace, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
