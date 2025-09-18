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

// Deployment extends the k8s deployment.
type Deployment struct {
	client v1.DeploymentInterface
	appsv1.Deployment
}

// NewDeployment creates a new deployment resource.
func NewDeployment(client v1.DeploymentInterface, namespace string, name string, in *appsv1.Deployment) *Deployment {
	if in == nil {
		in = &appsv1.Deployment{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &Deployment{client, *in}
}

func (d *Deployment) DeleteOrphan(ctx context.Context) error {
	return nil
}

// Clone returns a copy of the resource.
func (d *Deployment) Clone() Resource {
	clone := *d
	return &clone
}

// ID returns a comparable ID for the Resource.
func (d *Deployment) ID() ID {
	return ID{"Deployment", d.Name, d.Namespace}
}

// Apply patches the resource change.
func (d *Deployment) Apply(ctx context.Context) error {
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &d.Deployment)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := d.client.Patch(ctx, d.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = d.client.Create(ctx, &d.Deployment, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "deployment %q", d.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	d.Deployment = *res
	return nil
}

// Get refreshes the resource.
func (d *Deployment) Get(ctx context.Context) error {
	res, err := d.client.Get(ctx, d.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	d.Deployment = *res
	return nil
}

// Delete removes the resource.
func (d *Deployment) Delete(ctx context.Context) error {
	err := d.client.Delete(ctx, d.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s deployment for deletion")
	}
	return errors.Trace(err)
}

// ComputeStatus returns a juju status for the resource.
func (d *Deployment) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if d.DeletionTimestamp != nil {
		return "", status.Terminated, d.DeletionTimestamp.Time, nil
	}
	if d.Status.ReadyReplicas == d.Status.Replicas {
		return "", status.Active, now, nil
	}
	return "", status.Waiting, now, nil
}

// ListDeployments returns a list of deployments.
func ListDeployments(ctx context.Context, client v1.DeploymentInterface, namespace string, opts metav1.ListOptions) ([]Deployment, error) {
	var items []Deployment
	for {
		res, err := client.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, *NewDeployment(client, namespace, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
