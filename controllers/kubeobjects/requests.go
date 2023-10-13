// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package kubeobjects

import (
	"context"

	"github.com/go-logr/logr"
	ramen "github.com/ramendr/ramen/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type (
	ProtectRequest interface{ Request }
	RecoverRequest interface{ Request }
)

type Request interface {
	Object() client.Object
	Name() string
	StartTime() metav1.Time
	EndTime() metav1.Time
	Status(logr.Logger) error
	Deallocate(context.Context, client.Writer, logr.Logger) error
}

type Requests interface {
	Count() int
	Get(i int) Request
}

func RequestsMapKeyedByName(requestsStruct Requests) map[string]Request {
	requests := make(map[string]Request, requestsStruct.Count())

	for i := 0; i < requestsStruct.Count(); i++ {
		request := requestsStruct.Get(i)
		requests[request.Name()] = request
	}

	return requests
}

type RequestProcessingError struct{ string }

func RequestProcessingErrorCreate(s string) RequestProcessingError { return RequestProcessingError{s} }
func (e RequestProcessingError) Error() string                     { return e.string }
func (RequestProcessingError) Is(err error) bool                   { return true }

type RequestsManager interface {
	ProtectsPath() string
	RecoversPath() string
	ProtectRequestNew() ProtectRequest
	RecoverRequestNew() RecoverRequest
	ProtectRequestCreate(
		c context.Context, w client.Writer, l logr.Logger,
		s3Url string,
		s3BucketName string,
		s3RegionName string,
		s3KeyPrefix string,
		secretKeyRef *corev1.SecretKeySelector,
		caCertificates []byte,
		objectsSpec ramen.OperationSpec,
		requestNamespaceName string,
		protectRequestName string,
		labels map[string]string,
		annotations map[string]string,
	) (ProtectRequest, error)
	RecoverRequestCreate(
		c context.Context, w client.Writer, l logr.Logger,
		s3Url string,
		s3BucketName string,
		s3RegionName string,
		s3KeyPrefix string,
		secretKeyRef *corev1.SecretKeySelector,
		caCertificates []byte,
		recoverSpec ramen.RecoverSpec,
		requestNamespaceName string,
		protectRequestName string,
		protectRequest ProtectRequest,
		recoverRequestName string,
		labels map[string]string,
		annotations map[string]string,
	) (RecoverRequest, error)
	ProtectRequestsGet(
		c context.Context, r client.Reader, requestNamespaceName string, labels map[string]string,
	) (Requests, error)
	RecoverRequestsGet(
		c context.Context, r client.Reader, requestNamespaceName string, labels map[string]string,
	) (Requests, error)
	ProtectRequestsDelete(c context.Context, w client.Writer, requestNamespaceName string, labels map[string]string) error
	RecoverRequestsDelete(c context.Context, w client.Writer, requestNamespaceName string, labels map[string]string) error
}
