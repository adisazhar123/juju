// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// ClusterRole extends the k8s cluster role.
type ClusterRole struct {
	client rbacv1client.ClusterRoleInterface
	rbacv1.ClusterRole
}

func (r *ClusterRole) DeleteOrphan(ctx context.Context) error {
	return nil
}

// NewClusterRole creates a new cluster role resource.
func NewClusterRole(client rbacv1client.ClusterRoleInterface, name string, in *rbacv1.ClusterRole) *ClusterRole {
	if in == nil {
		in = &rbacv1.ClusterRole{}
	}
	in.SetName(name)
	return &ClusterRole{client, *in}
}

// Clone returns a copy of the resource.
func (r *ClusterRole) Clone() Resource {
	clone := *r
	return &clone
}

// ID returns a comparable ID for the Resource.
func (r *ClusterRole) ID() ID {
	return ID{"ClusterRole", r.Name, r.Namespace}
}

// Apply patches the resource change.
func (r *ClusterRole) Apply(ctx context.Context) error {
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &r.ClusterRole)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := r.client.Patch(ctx, r.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = r.client.Create(ctx, &r.ClusterRole, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "cluster role %q", r.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	r.ClusterRole = *res
	return nil
}

// Get refreshes the resource.
func (r *ClusterRole) Get(ctx context.Context) error {
	res, err := r.client.Get(ctx, r.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	r.ClusterRole = *res
	return nil
}

// Delete removes the resource.
func (r *ClusterRole) Delete(ctx context.Context) error {
	err := r.client.Delete(ctx, r.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s cluster role for deletion")
	}
	return errors.Trace(err)
}

// Ensure ensures this cluster role exists in it's desired form inside the
// cluster. If the object does not exist it's updated and if the object exists
// it's updated. The method also takes an optional set of claims to test the
// exisiting Kubernetes object with to assert ownership before overwriting it.
func (r *ClusterRole) Ensure(
	ctx context.Context,
	claims ...Claim,
) ([]func(), error) {
	// TODO(caas): roll this into Apply()
	cleanups := []func(){}
	hasClaim := true

	existing := ClusterRole{r.client, r.ClusterRole}
	err := existing.Get(ctx)
	if err == nil {
		hasClaim, err = RunClaims(claims...).Assert(&existing.ClusterRole)
	}
	if err != nil && !errors.IsNotFound(err) {
		return cleanups, errors.Annotatef(
			err,
			"checking for existing cluster role %q",
			existing.ClusterRole.Name,
		)
	}

	if !hasClaim {
		return cleanups, errors.AlreadyExistsf(
			"cluster role %q not controlled by juju", r.Name)
	}

	cleanups = append(cleanups, func() { _ = r.Delete(ctx) })
	if errors.IsNotFound(err) {
		return cleanups, r.Apply(ctx)
	}

	if err := r.Update(ctx); err != nil {
		return cleanups, err
	}
	return cleanups, nil
}

// ComputeStatus returns a juju status for the resource.
func (r *ClusterRole) ComputeStatus(_ context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if r.DeletionTimestamp != nil {
		return "", status.Terminated, r.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// Update updates the object in the Kubernetes cluster to the new representation
func (r *ClusterRole) Update(ctx context.Context) error {
	out, err := r.client.Update(
		ctx,
		&r.ClusterRole,
		metav1.UpdateOptions{
			FieldManager: JujuFieldManager,
		},
	)
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "updating cluster role")
	} else if err != nil {
		return errors.Trace(err)
	}
	r.ClusterRole = *out
	return nil
}

// ListClusterRoles returns a list of cluster roles.
func ListClusterRoles(ctx context.Context, client rbacv1client.ClusterRoleInterface, opts metav1.ListOptions) ([]ClusterRole, error) {
	var items []ClusterRole
	for {
		res, err := client.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, *NewClusterRole(client, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
