package e2e

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	helloWorldHelmAddonName = "helloworldhelm"
	addonInstallNamespace   = "open-cluster-management-agent-addon"

	imageConfigName    = "image-config"
	overrideImageValue = "quay.io/ocm/addon-examples:latest"
)

var _ = ginkgo.Describe("install/uninstall helloworld helm addons", func() {
	var helloworldhelmAddon = addonapiv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: helloWorldHelmAddonName,
		},
		Spec: addonapiv1alpha1.ManagedClusterAddOnSpec{},
	}

	ginkgo.BeforeEach(func() {
		gomega.Eventually(func() error {
			_, err := hubClusterClient.ClusterV1().ManagedClusters().Get(context.Background(), managedClusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			_, err = hubKubeClient.CoreV1().Namespaces().Get(context.Background(), managedClusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			_, err = hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.Background(), helloWorldHelmAddonName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					_, cerr := hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Create(context.Background(), &helloworldhelmAddon, metav1.CreateOptions{})
					if cerr != nil {
						return cerr
					}
				}
				return err
			}

			ginkgo.By("Make sure the agent namespace is created")
			testNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: agentInstallNamespaceConfig,
				},
			}
			_, err = hubKubeClient.CoreV1().Namespaces().Create(context.Background(), testNs, metav1.CreateOptions{})
			if err != nil {
				if errors.IsAlreadyExists(err) {
					ginkgo.By("The agent namespace is already created")
					return nil
				}
				return err
			}
			ginkgo.By("The agent namespace is created")

			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		ginkgo.By("Clean up the mca after each case.")
		gomega.Eventually(func() error {
			_, err := hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(
				context.Background(), helloWorldHelmAddonName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					// only return nil if the addon is deleted
					return nil
				}
				return err
			}

			err = hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Delete(
				context.Background(), helloWorldHelmAddonName, metav1.DeleteOptions{})
			if err != nil {
				return err
			}

			return fmt.Errorf("addon %s/%s is not deleted", managedClusterName, helloWorldHelmAddonName)
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Clean up the orphan agent after each case.")
		// delete the deployment since it has the deletion-orphan annotation, otherwise it may affect the
		// test result of each case
		gomega.Eventually(func() error {
			agentDeploymentName := "helloworldhelm-agent"
			_, err := hubKubeClient.AppsV1().Deployments(addonInstallNamespace).Get(
				context.Background(), agentDeploymentName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					// only return nil if the agent deployment is deleted
					return nil
				}
				return err
			}

			err = hubKubeClient.AppsV1().Deployments(addonInstallNamespace).Delete(
				context.Background(), agentDeploymentName, metav1.DeleteOptions{})
			if err != nil {
				return err
			}

			return fmt.Errorf("addon agent deployment %s/%s is not deleted", addonInstallNamespace, agentDeploymentName)
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Clean up the customized agent install namespace after each case.")
		gomega.Eventually(func() error {
			_, err := hubKubeClient.CoreV1().Namespaces().Get(context.Background(),
				agentInstallNamespaceConfig, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return nil
			}

			if err == nil {
				errd := hubKubeClient.CoreV1().Namespaces().Delete(context.Background(),
					agentInstallNamespaceConfig, metav1.DeleteOptions{})
				if errd != nil && !errors.IsNotFound(errd) {
					return errd
				}
				return fmt.Errorf("ns is deleting, need re-check if namespace is not found")
			}

			return err
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Clean up CSRs for addon: %s", helloWorldHelmAddonName))
		_ = cleanupCSR(hubKubeClient, helloWorldHelmAddonName)
	})

	ginkgo.It("addon should be available", func() {
		ginkgo.By("Make sure addon is available and has pre-delete finalizer")
		gomega.Eventually(func() error {
			addon, err := hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.Background(), helloWorldHelmAddonName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			hasPreDeleteFinalizer := false
			for _, f := range addon.Finalizers {
				if f == addonapiv1alpha1.AddonPreDeleteHookFinalizer {
					hasPreDeleteFinalizer = true
				}
			}
			if !hasPreDeleteFinalizer {
				return fmt.Errorf("expected pre delete hook finalizer")
			}

			if !meta.IsStatusConditionTrue(addon.Status.Conditions, "ManifestApplied") {
				return fmt.Errorf("addon should be applied to spoke, but get condition %v", addon.Status.Conditions)
			}

			if !meta.IsStatusConditionTrue(addon.Status.Conditions, "Available") {
				return fmt.Errorf("addon should be available on spoke, but get condition %v", addon.Status.Conditions)
			}

			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Make sure addon is functioning")
		configmap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("config-%s", rand.String(6)),
				Namespace: managedClusterName,
			},
			Data: map[string]string{
				"key1": rand.String(6),
				"key2": rand.String(6),
			},
		}

		_, err := hubKubeClient.CoreV1().ConfigMaps(managedClusterName).Create(context.Background(), configmap, metav1.CreateOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Eventually(func() error {
			copyiedConfig, err := hubKubeClient.CoreV1().ConfigMaps(addonInstallNamespace).Get(context.Background(), configmap.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}

			if !equality.Semantic.DeepEqual(copyiedConfig.Data, configmap.Data) {
				return fmt.Errorf("expected configmap is not correct, %v", copyiedConfig.Data)
			}
			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("The pre-delete job should clean up the configmap after the addon is deleted")
		err = hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Delete(context.Background(), helloWorldHelmAddonName, metav1.DeleteOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Eventually(func() error {
			_, err := hubKubeClient.CoreV1().ConfigMaps(addonInstallNamespace).Get(context.Background(), configmap.Name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return err
			}

			return fmt.Errorf("the configmap should be deleted")
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		gomega.Eventually(func() error {
			_, err := hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.Background(), helloWorldHelmAddonName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return err
			}

			return fmt.Errorf("the managedClusterAddon should be deleted")
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("The pre-delete job should be deleted ")
		gomega.Eventually(func() error {
			_, err := hubKubeClient.BatchV1().Jobs(addonInstallNamespace).Get(context.Background(), "helloworldhelm-cleanup-configmap", metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return err
			}

			return fmt.Errorf("the job should be deleted")
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("The deployment with deletion-orphan annotation should not be deleted ")
		gomega.Eventually(func() error {
			_, err := hubKubeClient.AppsV1().Deployments(addonInstallNamespace).Get(context.Background(), "helloworldhelm-agent", metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return fmt.Errorf("the deployment should not be deleted")
				}
				return err
			}
			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("addon should be configured with multiple configurations", func() {
		ginkgo.By("Prepare a AddOnDeploymentConfig for addon nodeSelector and tolerations")
		gomega.Eventually(func() error {
			return prepareAddOnDeploymentConfig(managedClusterName)
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Prepare a ConfigMap for addon image configuration")
		gomega.Eventually(func() error {
			_, err := hubKubeClient.CoreV1().ConfigMaps(managedClusterName).Get(context.Background(), imageConfigName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				if _, err := hubKubeClient.CoreV1().ConfigMaps(managedClusterName).Create(
					context.Background(),
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      imageConfigName,
							Namespace: managedClusterName,
						},
						Data: map[string]string{"image": overrideImageValue, "imagePullPolicy": "Never"},
					},
					metav1.CreateOptions{},
				); err != nil {
					return err
				}
			}
			if err != nil {
				return err
			}

			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Add the configs to ManagedClusterAddOn")
		gomega.Eventually(func() error {
			addon, err := hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.Background(), helloWorldHelmAddonName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			newAddon := addon.DeepCopy()
			newAddon.SetAnnotations(map[string]string{
				"addon.open-cluster-management.io/values": `{"global":{"imagePullSecret":"mySecret"}}`,
			})
			newAddon.Spec.Configs = []addonapiv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Resource: "configmaps",
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Namespace: managedClusterName,
						Name:      imageConfigName,
					},
				},
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    "addon.open-cluster-management.io",
						Resource: "addondeploymentconfigs",
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Namespace: managedClusterName,
						Name:      deployConfigName,
					},
				},
			}
			_, err = hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Update(context.Background(), newAddon, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Make sure addon is configured")
		gomega.Eventually(func() error {
			agentDeploy, err := hubKubeClient.AppsV1().Deployments(addonInstallNamespace).Get(context.Background(), "helloworldhelm-agent", metav1.GetOptions{})
			if err != nil {
				return err
			}

			imagePullSecrets := agentDeploy.Spec.Template.Spec.ImagePullSecrets
			if len(imagePullSecrets) != 1 {
				return fmt.Errorf("expect one image pull secret, but %v", imagePullSecrets)
			}
			if imagePullSecrets[0].Name != "mySecret" {
				return fmt.Errorf("the imagePullSecret is not overriden by the value in annotion, %v", imagePullSecrets)
			}

			containers := agentDeploy.Spec.Template.Spec.Containers
			if len(containers) != 1 {
				return fmt.Errorf("expect one container, but %v", containers)
			}

			if containers[0].Image != overrideImageValue {
				return fmt.Errorf("unexpected image %s", containers[0].Image)
			}

			if containers[0].ImagePullPolicy != "Never" {
				return fmt.Errorf("unexpected image pull policy  %s", containers[0].ImagePullPolicy)
			}

			if !equality.Semantic.DeepEqual(agentDeploy.Spec.Template.Spec.NodeSelector, nodeSelector) {
				return fmt.Errorf("unexpected nodeSeletcor %v", agentDeploy.Spec.Template.Spec.NodeSelector)
			}

			if !equality.Semantic.DeepEqual(agentDeploy.Spec.Template.Spec.Tolerations, tolerations) {
				return fmt.Errorf("unexpected tolerations %v", agentDeploy.Spec.Template.Spec.Tolerations)
			}

			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("addon image override should be configured by addonDeploymentConfig, and it takes precedence over the managed cluster annotation", func() {
		ginkgo.By("Prepare cluster annotation for addon image override config")
		overrideRegistries := addonapiv1alpha1.AddOnDeploymentConfigSpec{
			// should be different from the registries in the addonDeploymentConfig
			Registries: []addonapiv1alpha1.ImageMirror{
				{
					Source: "quay.io/open-cluster-management/addon-examples",
					Mirror: "quay.io/ocm/addon-examples-test",
				},
			},
		}
		registriesJson, err := json.Marshal(overrideRegistries)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(
				context.Background(), managedClusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			newCluster := cluster.DeepCopy()

			annotations := cluster.Annotations
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[clusterv1.ClusterImageRegistriesAnnotationKey] = string(registriesJson)

			newCluster.Annotations = annotations
			_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(
				context.Background(), newCluster, metav1.UpdateOptions{})
			return err
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Prepare a AddOnDeploymentConfig for addon image override config")
		gomega.Eventually(func() error {
			return prepareImageOverrideAddOnDeploymentConfig(managedClusterName)
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Add the configs to ManagedClusterAddOn")
		gomega.Eventually(func() error {
			addon, err := hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(
				context.Background(), helloWorldHelmAddonName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			newAddon := addon.DeepCopy()
			newAddon.Spec.Configs = []addonapiv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    "addon.open-cluster-management.io",
						Resource: "addondeploymentconfigs",
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Namespace: managedClusterName,
						Name:      deployImageOverrideConfigName,
					},
				},
			}
			_, err = hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Update(
				context.Background(), newAddon, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Make sure addon is configured")
		gomega.Eventually(func() error {
			agentDeploy, err := hubKubeClient.AppsV1().Deployments(addonInstallNamespace).Get(
				context.Background(), "helloworldhelm-agent", metav1.GetOptions{})
			if err != nil {
				return err
			}

			containers := agentDeploy.Spec.Template.Spec.Containers
			if len(containers) != 1 {
				return fmt.Errorf("expect one container, but %v", containers)
			}

			if containers[0].Image != overrideImageValue {
				return fmt.Errorf("unexpected image %s", containers[0].Image)
			}

			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		// restore the image override config, because it may affect other test cases
		ginkgo.By("Restore the configs to ManagedClusterAddOn")
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(
				context.Background(), managedClusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			newCluster := cluster.DeepCopy()
			delete(newCluster.Annotations, clusterv1.ClusterImageRegistriesAnnotationKey)
			_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(
				context.Background(), newCluster, metav1.UpdateOptions{})
			return err
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("addon should be configured by managed cluster annotation for image override", func() {
		ginkgo.By("Prepare cluster annotation for addon image override config")
		overrideRegistries := addonapiv1alpha1.AddOnDeploymentConfigSpec{
			Registries: registries,
		}
		registriesJson, err := json.Marshal(overrideRegistries)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(
				context.Background(), managedClusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			newCluster := cluster.DeepCopy()

			annotations := cluster.Annotations
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[clusterv1.ClusterImageRegistriesAnnotationKey] = string(registriesJson)

			newCluster.Annotations = annotations
			_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(
				context.Background(), newCluster, metav1.UpdateOptions{})
			return err
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Make sure addon is configured")
		gomega.Eventually(func() error {
			agentDeploy, err := hubKubeClient.AppsV1().Deployments(addonInstallNamespace).Get(
				context.Background(), "helloworldhelm-agent", metav1.GetOptions{})
			if err != nil {
				return err
			}

			containers := agentDeploy.Spec.Template.Spec.Containers
			if len(containers) != 1 {
				return fmt.Errorf("expect one container, but %v", containers)
			}

			if containers[0].Image != overrideImageValue {
				return fmt.Errorf("unexpected image %s", containers[0].Image)
			}

			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		// restore the image override config, because it may affect other test cases
		ginkgo.By("Restore the configs to ManagedClusterAddOn")
		gomega.Eventually(func() error {
			cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(
				context.Background(), managedClusterName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			newCluster := cluster.DeepCopy()
			delete(newCluster.Annotations, clusterv1.ClusterImageRegistriesAnnotationKey)
			_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(
				context.Background(), newCluster, metav1.UpdateOptions{})
			return err
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("addon should pick up the proxy settings from addondeploymentconfig", func() {
		ginkgo.By("Prepare a AddOnDeploymentConfig for proxy settings")
		gomega.Eventually(func() error {
			return prepareProxyConfigAddOnDeploymentConfig(managedClusterName)
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Add the configs to ManagedClusterAddOn")
		gomega.Eventually(func() error {
			addon, err := hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(
				context.Background(), helloWorldHelmAddonName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			newAddon := addon.DeepCopy()
			newAddon.Spec.Configs = []addonapiv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    "addon.open-cluster-management.io",
						Resource: "addondeploymentconfigs",
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Namespace: managedClusterName,
						Name:      deployProxyConfigName,
					},
				},
			}
			_, err = hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Update(
				context.Background(), newAddon, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Make sure addon is configured")
		gomega.Eventually(func() error {
			agentDeploy, err := hubKubeClient.AppsV1().Deployments(addonInstallNamespace).Get(
				context.Background(), "helloworldhelm-agent", metav1.GetOptions{})
			if err != nil {
				return err
			}

			containers := agentDeploy.Spec.Template.Spec.Containers
			if len(containers) != 1 {
				return fmt.Errorf("expect one container, but %v", containers)
			}

			// check the proxy settings
			deployProxyConfig := addonapiv1alpha1.ProxyConfig{}
			for _, envVar := range containers[0].Env {
				if envVar.Name == "HTTP_PROXY" {
					deployProxyConfig.HTTPProxy = envVar.Value
				}

				if envVar.Name == "HTTPS_PROXY" {
					deployProxyConfig.HTTPSProxy = envVar.Value
				}

				if envVar.Name == "NO_PROXY" {
					deployProxyConfig.NoProxy = envVar.Value
				}
			}

			if !equality.Semantic.DeepEqual(proxyConfig, deployProxyConfig) {
				return fmt.Errorf("expected proxy settings %v, but got %v", proxyConfig, deployProxyConfig)
			}

			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("addon should adopt the resource requirements from addondeploymentconfig", func() {
		ginkgo.By("Prepare a AddOnDeploymentConfig for resource requirement settings")
		gomega.Eventually(func() error {
			return prepareResourceRequirementsAddOnDeploymentConfig(managedClusterName)
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Add the configs to ManagedClusterAddOn")
		gomega.Eventually(func() error {
			addon, err := hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(
				context.Background(), helloWorldHelmAddonName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			newAddon := addon.DeepCopy()
			newAddon.Spec.Configs = []addonapiv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    "addon.open-cluster-management.io",
						Resource: "addondeploymentconfigs",
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Namespace: managedClusterName,
						Name:      deployResourceRequirementConfigName,
					},
				},
			}
			_, err = hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Update(
				context.Background(), newAddon, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Make sure addon is configured")
		gomega.Eventually(func() error {
			agentDeploy, err := hubKubeClient.AppsV1().Deployments(addonInstallNamespace).Get(
				context.Background(), "helloworldhelm-agent", metav1.GetOptions{})
			if err != nil {
				return err
			}

			containers := agentDeploy.Spec.Template.Spec.Containers
			if len(containers) != 1 {
				return fmt.Errorf("expect one container, but %v", containers)
			}

			// check the resource requirements
			required := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("32Mi"),
				},
			}

			if !equality.Semantic.DeepEqual(containers[0].Resources, required) {
				return fmt.Errorf("expected resource requirement %v, but got %v", required, containers[0].Resources)
			}

			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
	})

	ginkgo.It("addon registration agent install namespace should work", func() {
		ginkgo.By("Prepare a AddOnDeploymentConfig for addon agent install namespace")
		gomega.Eventually(func() error {
			return prepareAgentInstallNamespaceAddOnDeploymentConfig(managedClusterName)
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Add the configs to ManagedClusterAddOn")
		gomega.Eventually(func() error {
			addon, err := hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(
				context.Background(), helloWorldHelmAddonName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			newAddon := addon.DeepCopy()
			newAddon.Spec.Configs = []addonapiv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    "addon.open-cluster-management.io",
						Resource: "addondeploymentconfigs",
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Namespace: managedClusterName,
						Name:      deployAgentInstallNamespaceConfigName,
					},
				},
			}
			_, err = hubAddOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Update(
				context.Background(), newAddon, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return nil
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Make sure addon is configured")
		gomega.Eventually(func() error {
			_, err := hubKubeClient.CoreV1().Secrets(agentInstallNamespaceConfig).Get(
				context.Background(), "helloworldhelm-hub-kubeconfig", metav1.GetOptions{})
			return err
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		gomega.Eventually(func() error {
			_, err := hubKubeClient.AppsV1().Deployments(agentInstallNamespaceConfig).Get(
				context.Background(), "helloworldhelm-agent", metav1.GetOptions{})
			return err
		}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
	})

})
