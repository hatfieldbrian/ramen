/*
Copyright 2021 The RamenDR authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	config "k8s.io/component-base/config/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controller_runtime_config "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	volrep "github.com/csi-addons/volume-replication-operator/api/v1alpha1"
	ocmclv1 "github.com/open-cluster-management/api/cluster/v1"
	ocmworkv1 "github.com/open-cluster-management/api/work/v1"
	cpcv1 "github.com/stolostron/config-policy-controller/api/v1"
	gppv1 "github.com/stolostron/governance-policy-propagator/api/v1"
	viewv1beta1 "github.com/stolostron/multicloud-operators-foundation/pkg/apis/view/v1beta1"
	plrv1 "github.com/stolostron/multicloud-operators-placementrule/pkg/apis/apps/v1"

	ramendrv1alpha1 "github.com/ramendr/ramen/api/v1alpha1"
	ramencontrollers "github.com/ramendr/ramen/controllers"
	"github.com/ramendr/ramen/controllers/util"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg         *rest.Config
	apiReader   client.Reader
	k8sClient   client.Client
	testEnv     *envtest.Environment
	configMap   *corev1.ConfigMap
	ramenConfig *ramendrv1alpha1.RamenConfig
	testLog     logr.Logger

	timeout  = time.Second * 10
	interval = time.Millisecond * 10

	plRuleNames []string

	s3Secrets     [1]corev1.Secret
	s3Profiles    [6]ramendrv1alpha1.S3StoreProfile
	objectStorers [2]ramencontrollers.ObjectStorer

	ramenNamespace = "ns-envtest"
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

func createOperatorNamespace(ramenNamespace string) {
	ramenNamespaceLookupKey := types.NamespacedName{Name: ramenNamespace}
	ramenNamespaceObj := &corev1.Namespace{}

	err := k8sClient.Get(context.TODO(), ramenNamespaceLookupKey, ramenNamespaceObj)
	if err != nil {
		ramenNamespaceObj = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ramenNamespace},
		}
		Expect(k8sClient.Create(context.TODO(), ramenNamespaceObj)).NotTo(HaveOccurred(),
			"failed to create operator namespace")
	}
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	testLog = ctrl.Log.WithName("tester")
	testLog.Info("Starting the controller test suite", "time", time.Now())

	// default controller type to DRHubType
	ramencontrollers.ControllerType = ramendrv1alpha1.DRHubType

	if _, set := os.LookupEnv("KUBEBUILDER_ASSETS"); !set {
		Expect(os.Setenv("KUBEBUILDER_ASSETS", "../testbin/bin")).To(Succeed())
	}

	rNs, set := os.LookupEnv("POD_NAMESPACE")
	if !set {
		Expect(os.Setenv("POD_NAMESPACE", ramenNamespace)).To(Succeed())
	} else {
		ramenNamespace = rNs
	}

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd", "bases"),
			filepath.Join("..", "hack", "test"),
		},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = ocmworkv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = ocmclv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = plrv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = viewv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = cpcv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = gppv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = ramendrv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = volrep.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	createOperatorNamespace(ramenNamespace)
	ramenConfig = &ramendrv1alpha1.RamenConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RamenConfig",
			APIVersion: ramendrv1alpha1.GroupVersion.String(),
		},
		ControllerManagerConfigurationSpec: controller_runtime_config.ControllerManagerConfigurationSpec{
			LeaderElection: &config.LeaderElectionConfiguration{
				LeaderElect:  new(bool),
				ResourceName: ramencontrollers.HubLeaderElectionResourceName,
			},
		},
		RamenControllerType: ramendrv1alpha1.DRHubType,
	}
	ramenConfig.DrClusterOperator.DeploymentAutomationEnabled = true
	ramenConfig.DrClusterOperator.S3SecretDistributionEnabled = true
	configMap, err = ramencontrollers.ConfigMapNew(
		ramenNamespace,
		ramencontrollers.HubOperatorConfigMapName,
		ramenConfig,
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Create(context.TODO(), configMap)).To(Succeed())

	s3Secrets[0] = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: configMap.Namespace, Name: "s3secret0"},
		StringData: map[string]string{
			"AWS_ACCESS_KEY_ID":     awsAccessKeyIDSucc,
			"AWS_SECRET_ACCESS_KEY": "",
		},
	}
	s3ProfileNew := func(profileNameSuffix, bucketName string) ramendrv1alpha1.S3StoreProfile {
		return ramendrv1alpha1.S3StoreProfile{
			S3ProfileName:        "s3profile" + profileNameSuffix,
			S3Bucket:             bucketName,
			S3CompatibleEndpoint: "http://192.168.39.223:30000",
			S3Region:             "us-east-1",
			S3SecretRef:          corev1.SecretReference{Name: s3Secrets[0].Name},
		}
	}

	s3Profiles[0] = s3ProfileNew("0", bucketNameSucc)
	s3Profiles[1] = s3ProfileNew("1", bucketNameSucc2)
	s3Profiles[2] = s3ProfileNew("2", bucketNameFail)
	s3Profiles[3] = s3ProfileNew("3", bucketNameFail2)
	s3Profiles[4] = s3ProfileNew("4", bucketListFail)

	s3SecretsPolicyNamesSet := func() {
		for idx := range s3Secrets {
			_, _, v, _ := util.GeneratePolicyResourceNames(s3Secrets[idx].Name)
			plRuleNames = append(plRuleNames, v)
		}
	}
	s3SecretCreate := func(s3Secret *corev1.Secret) {
		Expect(k8sClient.Create(context.TODO(), s3Secret)).To(Succeed())
	}
	s3SecretsCreate := func() {
		for i := range s3Secrets {
			s3SecretCreate(&s3Secrets[i])
		}
	}
	s3ProfilesSecretNamespaceNameSet := func() {
		namespaceName := s3Secrets[0].Namespace
		for i := range s3Profiles {
			s3Profiles[i].S3SecretRef.Namespace = namespaceName
		}
	}
	s3Profiles[5] = ramendrv1alpha1.S3StoreProfile{
		S3ProfileName:        "drc-s3profile",
		S3Bucket:             bucketNameSucc,
		S3CompatibleEndpoint: "http://192.168.39.223:30000",
		S3Region:             "us-east-1",
		S3SecretRef:          corev1.SecretReference{Name: s3Secrets[0].Name},
	}
	s3ProfilesUpdate := func() {
		s3ProfilesStore(s3Profiles[0:])
	}
	fakeObjectStorerGet := func(i int) ramencontrollers.ObjectStorer {
		objectStorer, err := fakeObjectStoreGetter{}.ObjectStore(
			context.TODO(), apiReader, s3Profiles[i].S3ProfileName, "", testLog,
		)
		Expect(err).To(BeNil())

		return objectStorer
	}
	objectStorersSet := func() {
		for i := range s3Profiles[:len(objectStorers)] {
			objectStorers[i] = fakeObjectStorerGet(i)
		}
	}
	s3SecretsPolicyNamesSet()
	s3SecretsCreate()
	s3ProfilesSecretNamespaceNameSet()
	s3ProfilesUpdate()

	options, err := manager.Options{Scheme: scheme.Scheme}.AndFrom(ramenConfig)
	Expect(err).NotTo(HaveOccurred())

	// test controller behavior
	k8sManager, err := ctrl.NewManager(cfg, options)
	Expect(err).ToNot(HaveOccurred())

	Expect((&ramencontrollers.DRClusterReconciler{
		Client:            k8sManager.GetClient(),
		APIReader:         k8sManager.GetAPIReader(),
		Scheme:            k8sManager.GetScheme(),
		ObjectStoreGetter: fakeObjectStoreGetter{},
	}).SetupWithManager(k8sManager)).To(Succeed())

	Expect((&ramencontrollers.DRPolicyReconciler{
		Client:            k8sManager.GetClient(),
		APIReader:         k8sManager.GetAPIReader(),
		Scheme:            k8sManager.GetScheme(),
		ObjectStoreGetter: fakeObjectStoreGetter{},
	}).SetupWithManager(k8sManager)).To(Succeed())

	err = (&ramencontrollers.VolumeReplicationGroupReconciler{
		Client:         k8sManager.GetClient(),
		APIReader:      k8sManager.GetAPIReader(),
		Log:            ctrl.Log.WithName("controllers").WithName("VolumeReplicationGroup"),
		ObjStoreGetter: fakeObjectStoreGetter{},
		Scheme:         k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	drpcReconciler := (&ramencontrollers.DRPlacementControlReconciler{
		Client:    k8sManager.GetClient(),
		APIReader: k8sManager.GetAPIReader(),
		Log:       ctrl.Log.WithName("controllers").WithName("DRPlacementControl"),
		MCVGetter: FakeMCVGetter{},
		Scheme:    k8sManager.GetScheme(),
		Callback:  FakeProgressCallback,
	})
	err = drpcReconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())
	apiReader = k8sManager.GetAPIReader()
	Expect(apiReader).ToNot(BeNil())
	objectStorersSet()
}, 60)

var _ = AfterSuite(func() {
	Expect(k8sClient.Delete(context.TODO(), configMap)).To(Succeed())
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
