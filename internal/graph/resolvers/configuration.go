package resolvers

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	extv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	pkgv1 "github.com/crossplane/crossplane/apis/pkg/v1"

	"github.com/upbound/xgql/internal/graph/model"
	"github.com/upbound/xgql/internal/token"
)

type configuration struct {
	clients ClientCache
}

func (r *configuration) Events(ctx context.Context, obj *model.Configuration, limit *int) (*model.EventConnection, error) {
	return nil, nil
}

func (r *configuration) Revisions(ctx context.Context, obj *model.Configuration, limit *int, active *bool) (*model.ConfigurationRevisionConnection, error) { //nolint:gocyclo
	// NOTE(negz): This method is a little over our complexity goal. Be wary of
	// making it more complex.

	t, _ := token.FromContext(ctx)

	c, err := r.clients.Get(t)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get client")
	}

	in := &pkgv1.ConfigurationRevisionList{}
	if err := c.List(ctx, in); err != nil {
		return nil, errors.Wrap(err, "cannot list configurations")
	}

	out := &model.ConfigurationRevisionConnection{
		Items: make([]model.ConfigurationRevision, 0),
	}

	for i := range in.Items {
		cr := in.Items[i] // So we don't take the address of a range variable.

		// We're not the controller reference of this ConfigurationRevision;
		// it's not one of ours.
		// https://github.com/kubernetes/community/blob/0331e/contributors/design-proposals/api-machinery/controller-ref.md
		if c := metav1.GetControllerOf(&cr); c == nil || c.UID != types.UID(obj.Metadata.UID) {
			continue
		}

		// We only want the active PackageRevision, and this isn't it.
		if pointer.BoolPtrDerefOr(active, false) && cr.Spec.DesiredState != pkgv1.PackageRevisionActive {
			continue
		}

		out.Count++

		// We've hit our limit; we only want to count from hereon out.
		if limit != nil && *limit < out.Count {
			continue
		}

		i, err := model.GetConfigurationRevision(&cr)
		if err != nil {
			return nil, errors.Wrap(err, "cannot model configuration revision")
		}

		out.Items = append(out.Items, i)
	}

	return out, nil
}

type configurationRevision struct {
	clients ClientCache
}

func (r *configurationRevision) Events(ctx context.Context, obj *model.ConfigurationRevision, limit *int) (*model.EventConnection, error) {
	return nil, nil
}

type configurationRevisionStatus struct {
	clients ClientCache
}

func (r *configurationRevisionStatus) Objects(ctx context.Context, obj *model.ConfigurationRevisionStatus, limit *int) (*model.KubernetesResourceConnection, error) { //nolint:gocyclo
	// TODO(negz): This method is over our complexity goal. Maybe break the
	// switch out into its own function?

	t, _ := token.FromContext(ctx)

	c, err := r.clients.Get(t)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get client")
	}

	out := &model.KubernetesResourceConnection{
		Items: make([]model.KubernetesResource, 0, len(obj.ObjectRefs)),
	}

	for _, ref := range obj.ObjectRefs {
		// Crossplane lints configuration packages to ensure they only contain XRDs and Compositions
		// but this isn't enforced at the API level. We filter out anything that
		// isn't a CRD, just in case.
		if strings.Split(ref.APIVersion, "/")[0] != extv1.Group {
			continue
		}

		// Currently only two types exist in the apiextensions.crossplane.io API
		// group, so we assume that if we've found a resource in that group it
		// will be handled by the switch on ref.Kind below.
		out.Count++

		// We've hit our limit; we only want to count from hereon out.
		if limit != nil && *limit < out.Count {
			continue
		}

		switch ref.Kind {
		case extv1.CompositeResourceDefinitionKind:
			xrd := &extv1.CompositeResourceDefinition{}
			if err := c.Get(ctx, types.NamespacedName{Name: ref.Name}, xrd); err != nil {
				return nil, errors.Wrap(err, "cannot get CompositeResourceDefinition")
			}

			i, err := model.GetCompositeResourceDefinition(xrd)
			if err != nil {
				return nil, errors.Wrap(err, "cannot model composite resource definition")
			}

			out.Items = append(out.Items, i)
		case extv1.CompositionKind:
			cmp := &extv1.Composition{}
			if err := c.Get(ctx, types.NamespacedName{Name: ref.Name}, cmp); err != nil {
				return nil, errors.Wrap(err, "cannot get Composition")
			}

			i, err := model.GetComposition(cmp)
			if err != nil {
				return nil, errors.Wrap(err, "cannot model composition")
			}

			out.Items = append(out.Items, i)
		}

	}

	return out, nil
}
